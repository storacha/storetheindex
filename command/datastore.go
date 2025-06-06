package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	ddbsession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	dynamods "github.com/ipfs/go-ds-dynamodb"
	leveldb "github.com/ipfs/go-ds-leveldb"
	"github.com/ipni/storetheindex/config"
	"github.com/ipni/storetheindex/fsutil"
	s3ds "github.com/storacha/go-libstoracha/datastore/s3"
)

const (
	dsInfoPrefix = "/dsInfo/"
	dsVersionKey = "version"
	dsVersion    = "002"

	// updateBatchSize is the number of records to update at a time.
	updateBatchSize = 500000
)

func createDatastore(ctx context.Context, cfg config.Datastore) (datastore.Batching, string, error) {
	return createDS(ctx, cfg.Type, cfg.Dir, cfg.Region, false)
}

func createTmpDatastore(ctx context.Context, cfg config.Datastore) (datastore.Batching, string, error) {
	if cfg.TmpType == "" || cfg.TmpType == "none" {
		return nil, "", nil
	}

	return createDS(ctx, cfg.TmpType, cfg.TmpDir, cfg.TmpRegion, cfg.RemoveTmpAtStart)
}

func createDS(ctx context.Context, dsType, dirOrTable, region string, rmExisting bool) (datastore.Batching, string, error) {
	switch dsType {
	case "levelds":
		return createLevelDBDatastore(ctx, dirOrTable, rmExisting)
	case "dynamodb":
		return createDynamoDBDatastore(ctx, dirOrTable, region)
	case "s3":
		return createS3Datastore(ctx, dirOrTable, region)
	default:
		return nil, "", fmt.Errorf("only levelds and dynamodb datastore types supported, %q not supported", dsType)
	}
}

func createLevelDBDatastore(ctx context.Context, dir string, rmExisting bool) (datastore.Batching, string, error) {
	dataStorePath, err := config.Path("", dir)
	if err != nil {
		return nil, "", err
	}
	if rmExisting {
		if err = os.RemoveAll(dataStorePath); err != nil {
			return nil, "", fmt.Errorf("cannot remove temporary datastore directory: %w", err)
		}
	}
	if err = fsutil.DirWritable(dataStorePath); err != nil {
		return nil, "", err
	}
	ds, err := leveldb.NewDatastore(dataStorePath, nil)
	if err != nil {
		return nil, "", err
	}
	return ds, dataStorePath, nil
}

func createDynamoDBDatastore(ctx context.Context, tableName, tableRegion string) (datastore.Batching, string, error) {
	s := ddbsession.Must(ddbsession.NewSession(&aws.Config{
		Region: aws.String(tableRegion),
	}))
	c := dynamodb.New(s)
	ds := dynamods.New(c, tableName)

	return ds, tableName, nil
}

func createS3Datastore(ctx context.Context, bucketName, bucketRegion string) (datastore.Batching, string, error) {
	cfg := s3ds.Config{Bucket: bucketName, Region: bucketRegion}
	ds, err := s3ds.NewS3Datastore(cfg)
	if err != nil {
		return nil, "", err
	}

	return ds, bucketName, nil
}

func cleanupDTTempData(ctx context.Context, ds datastore.Batching) error {
	const dtCleanupTimeout = 10 * time.Minute
	const dtPrefix = "/data-transfer-v2"

	ctx, cancel := context.WithTimeout(ctx, dtCleanupTimeout)
	defer cancel()

	count, err := deletePrefix(ctx, ds, dtPrefix)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Info("Not enough time to finish data-transfer state cleanup")
			return ds.Sync(context.Background(), datastore.NewKey(dtPrefix))
		}
		return err
	}
	log.Infow("Removed old temporary data-transfer fsm records", "count", count)
	return nil
}

func updateDatastore(ctx context.Context, ds datastore.Batching) error {
	dsVerKey := datastore.NewKey(dsInfoPrefix + dsVersionKey)
	curVerData, err := ds.Get(ctx, dsVerKey)
	if err != nil && !errors.Is(err, datastore.ErrNotFound) {
		return fmt.Errorf("cannot check datastore: %w", err)
	}
	curVer := "000"
	if len(curVerData) != 0 {
		curVer = string(curVerData)
	}
	if curVer == dsVersion {
		return nil
	}
	if curVer > dsVersion {
		return fmt.Errorf("unknown datastore verssion: %s", curVer)
	}

	var count int

	log.Infof("Updating datastore from version %s to %s", curVer, dsVersion)
	if curVer < "001" {
		count, err = deletePrefix(ctx, ds, "/data-transfer-v2")
		if err != nil {
			return err
		}
		if count != 0 {
			log.Infow("Datastore update removed data-transfer fsm records", "count", count)
		}
		if err = rmOldTempRecords(ctx, ds); err != nil {
			return err
		}
	}
	if curVer < "002" {
		count, err = deletePrefix(ctx, ds, "/indexCounts")
		if err != nil {
			return err
		}
		if count != 0 {
			log.Infow("Datastore update removed index count records", "count", count)
		}
	}

	if err = ds.Put(ctx, dsVerKey, []byte(dsVersion)); err != nil {
		return err
	}
	if err = ds.Sync(ctx, datastore.NewKey("")); err != nil {
		return fmt.Errorf("cannot sync datastore: %w", err)
	}

	log.Infow("Datastore update finished")
	return nil
}

func rmOldTempRecords(ctx context.Context, ds datastore.Batching) error {
	q := query.Query{
		KeysOnly: true,
	}
	results, err := ds.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("cannot query datastore: %w", err)
	}
	defer results.Close()

	batch, err := ds.Batch(ctx)
	if err != nil {
		return fmt.Errorf("cannot create datastore batch: %w", err)
	}

	var cidCount, dtKeyCount, writeCount int
	for result := range results.Next() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if writeCount >= updateBatchSize {
			writeCount = 0
			if err = batch.Commit(ctx); err != nil {
				return fmt.Errorf("cannot commit datastore updates: %w", err)
			}
			log.Infow("Datastore update removed old records", "fsmData", dtKeyCount, "adData", cidCount)
		}
		if result.Error != nil {
			return fmt.Errorf("cannot read query result from datastore: %w", result.Error)
		}
		ent := result.Entry
		if len(ent.Key) == 0 {
			log.Warnf("result entry has empty key")
			continue
		}
		var key string
		if ent.Key[0] == '/' {
			key = ent.Key[1:]
		} else {
			key = ent.Key
		}

		before, after, found := strings.Cut(key, "/")
		if found {
			if before[0] >= '0' && before[0] <= '9' && len(after) > 22 {
				if err = batch.Delete(ctx, datastore.NewKey(key)); err != nil {
					return fmt.Errorf("cannot delete dt state key from datastore: %w", err)
				}
				writeCount++
				dtKeyCount++
			}
			continue
		}

		if _, err := cid.Decode(key); err != nil {
			log.Warnf("Unknown key: %s", key)
			continue
		}
		if err = batch.Delete(ctx, datastore.NewKey(key)); err != nil {
			return fmt.Errorf("cannot delete CID from datastore: %w", err)
		}
		writeCount++
		cidCount++
	}

	if err = batch.Commit(ctx); err != nil {
		return fmt.Errorf("cannot commit datastore updated: %w", err)
	}
	if err = ds.Sync(ctx, datastore.NewKey("")); err != nil {
		return fmt.Errorf("cannot sync datastore: %w", err)
	}

	if dtKeyCount != 0 || cidCount != 0 {
		log.Infow("Datastore update removed old records", "fsmData", dtKeyCount, "adData", cidCount)
	}
	return nil
}

func deletePrefix(ctx context.Context, ds datastore.Batching, prefix string) (int, error) {
	q := query.Query{
		KeysOnly: true,
		Prefix:   prefix,
	}
	results, err := ds.Query(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("cannot query datastore: %w", err)
	}
	defer results.Close()

	batch, err := ds.Batch(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot create datastore batch: %w", err)
	}

	var keyCount, writeCount int
	for result := range results.Next() {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if writeCount >= updateBatchSize {
			writeCount = 0
			if err = batch.Commit(ctx); err != nil {
				return 0, fmt.Errorf("cannot commit datastore: %w", err)
			}
			log.Infow("Removed datastore records", "count", keyCount)
		}
		if result.Error != nil {
			return 0, fmt.Errorf("cannot read query result from datastore: %w", result.Error)
		}
		ent := result.Entry
		if len(ent.Key) == 0 {
			log.Warnf("result entry has empty key")
			continue
		}

		if err = batch.Delete(ctx, datastore.NewKey(ent.Key)); err != nil {
			return 0, fmt.Errorf("cannot delete key from datastore: %w", err)
		}
		writeCount++
		keyCount++
	}

	if err = batch.Commit(ctx); err != nil {
		return 0, fmt.Errorf("cannot commit datastore: %w", err)
	}
	if err = ds.Sync(context.Background(), datastore.NewKey(q.Prefix)); err != nil {
		return 0, err
	}

	return keyCount, nil
}
