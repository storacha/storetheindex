package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ipni/go-indexer-core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
)

var _ indexer.Interface = (*ddbStore)(nil)

// Schema field names
const (
	// Providers table fields
	fieldProviderID = "ProviderID"
	fieldContextID  = "ContextID"
	fieldMetadata   = "Metadata"

	// Multihash map tablefields
	fieldMultihash = "Multihash"
	fieldValueKey  = "ValueKey"
)

// DynamoDBClient defines the interface for DynamoDB operations we use
type DynamoDBClient interface {
	dynamodb.QueryAPIClient
	dynamodb.BatchGetItemAPIClient
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

func NewClient(region string) DynamoDBClient {
	return dynamodb.NewFromConfig(aws.Config{
		Region: region,
	})
}

type ddbStore struct {
	client            DynamoDBClient
	providersTable    string
	multihashMapTable string
}

func NewStore(client DynamoDBClient, providersTable, multihashMapTable string) (*ddbStore, error) {
	return &ddbStore{client: client, providersTable: providersTable, multihashMapTable: multihashMapTable}, nil
}

// batchGetItems retrieves multiple items from the providers table in batches.
// TODO: Add support for handling unprocessed keys with retry logic.
func (s *ddbStore) batchGetItems(keys []map[string]types.AttributeValue) ([]map[string]types.AttributeValue, error) {
	const maxBatchSize = 100 // DynamoDB limit for BatchGetItem
	var allItems []map[string]types.AttributeValue

	// Process keys in batches of maxBatchSize
	for i := 0; i < len(keys); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]

		// Prepare the batch get request
		input := &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				s.providersTable: {
					Keys: batch,
				},
			},
		}

		// Execute the batch get request
		result, err := s.client.BatchGetItem(context.TODO(), input)
		if err != nil {
			return nil, fmt.Errorf("failed to batch get items: %w", err)
		}

		// Add the successfully retrieved items to our results
		if items, ok := result.Responses[s.providersTable]; ok {
			allItems = append(allItems, items...)
		}

		// TODO: Handle unprocessed keys (result.UnprocessedKeys)
	}

	return allItems, nil
}

// Get implements indexer.Interface.Get
func (s *ddbStore) Get(mh multihash.Multihash) ([]indexer.Value, bool, error) {
	// Query the multihashMapTable to get all ValueKeys for the given multihash
	result, err := s.client.Query(context.TODO(), &dynamodb.QueryInput{
		TableName:              aws.String(s.multihashMapTable),
		KeyConditionExpression: aws.String(fieldMultihash + " = :mh"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":mh": &types.AttributeValueMemberB{Value: mh},
		},
	})

	if err != nil {
		return nil, false, fmt.Errorf("failed to query multihash map table: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, false, nil
	}

	// Prepare batch get items request for providersTable
	var keys []map[string]types.AttributeValue
	for _, item := range result.Items {
		// ValueKey is in the format "providerID/contextID"
		valueKey, ok := item[fieldValueKey].(*types.AttributeValueMemberS)
		if !ok || valueKey == nil {
			continue
		}

		// Split the ValueKey to get providerID and contextID
		parts := strings.SplitN(valueKey.Value, "/", 2)
		if len(parts) != 2 {
			continue
		}

		keys = append(keys, map[string]types.AttributeValue{
			fieldProviderID: &types.AttributeValueMemberS{Value: parts[0]},
			fieldContextID:  &types.AttributeValueMemberB{Value: []byte(parts[1])},
		})
	}

	if len(keys) == 0 {
		return nil, false, nil
	}

	// Get provider items in batches, handling unprocessed keys
	providerItems, err := s.batchGetItems(keys)
	if err != nil {
		return nil, false, fmt.Errorf("failed to batch get from providers table: %w", err)
	}

	// Convert the results to indexer.Value slices
	// Note: If some provider/context combinations no longer exist in the providers table,
	// they won't be included in the results. This is expected behavior as we don't want
	// to return mappings to non-existent providers. The orphaned multihash mappings will
	// remain in the multihashes table but won't affect the correctness of the results.
	// TODO: Add a cleanup process to remove orphaned multihash mappings.
	var values []indexer.Value
	for _, item := range providerItems {
		providerID, err := peer.Decode(item[fieldProviderID].(*types.AttributeValueMemberS).Value)
		if err != nil {
			continue
		}

		var contextID, metadata []byte
		if ctxItem, ok := item[fieldContextID].(*types.AttributeValueMemberB); ok {
			contextID = ctxItem.Value
		}
		if metaItem, ok := item[fieldMetadata].(*types.AttributeValueMemberB); ok {
			metadata = metaItem.Value
		}

		values = append(values, indexer.Value{
			ProviderID:    providerID,
			ContextID:     contextID,
			MetadataBytes: metadata,
		})
	}

	return values, len(values) > 0, nil
}

// Put implements indexer.Interface.Put
func (s *ddbStore) Put(v indexer.Value, mhs ...multihash.Multihash) error {
	// First, update or create the provider record
	_, err := s.client.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(s.providersTable),
		Item: map[string]types.AttributeValue{
			fieldProviderID: &types.AttributeValueMemberS{Value: v.ProviderID.String()},
			fieldContextID:  &types.AttributeValueMemberB{Value: v.ContextID},
			fieldMetadata:   &types.AttributeValueMemberB{Value: v.MetadataBytes},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to put provider item: %w", err)
	}

	// If no multihashes provided, we're done
	if len(mhs) == 0 {
		return nil
	}

	// Prepare batch write items for multihashes mapping
	writeRequests := make([]types.WriteRequest, 0, len(mhs))
	for _, mh := range mhs {
		// Create a composite sort key: "providerID/contextID"
		valueKey := v.ProviderID.String() + "/" + string(v.ContextID)

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: map[string]types.AttributeValue{
					fieldMultihash: &types.AttributeValueMemberB{Value: mh},
					fieldValueKey:  &types.AttributeValueMemberS{Value: valueKey},
				},
			},
		})
	}

	// Write all requests in batches
	if err := s.batchWriteItems(writeRequests, s.multihashMapTable); err != nil {
		return fmt.Errorf("failed to batch write items: %w", err)
	}

	return nil
}

// batchWriteItems is a helper function to execute batch write operations.
// tableName specifies which table to perform the operations on.
func (s *ddbStore) batchWriteItems(requests []types.WriteRequest, tableName string) error {
	if len(requests) == 0 {
		return nil
	}

	// Split into chunks of 25 (DynamoDB limit)
	for i := 0; i < len(requests); i += 25 {
		end := i + 25
		if end > len(requests) {
			end = len(requests)
		}
		chunk := requests[i:end]

		_, err := s.client.BatchWriteItem(context.TODO(), &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				tableName: chunk,
			},
		})
		if err != nil {
			// Include the operation type in the error message for better debugging
			operationType := "write"
			if len(chunk) > 0 && chunk[0].DeleteRequest != nil {
				operationType = "delete"
			}
			return fmt.Errorf("batch %s failed: %w", operationType, err)
		}
	}

	return nil
}

// Remove implements indexer.Interface.Remove
func (s *ddbStore) Remove(v indexer.Value, mhs ...multihash.Multihash) error {
	// If no multihashes provided, nothing to do
	if len(mhs) == 0 {
		return nil
	}

	// Create the value key that we'll be removing
	valueKey := v.ProviderID.String() + "/" + string(v.ContextID)

	// Prepare batch delete requests
	deleteRequests := make([]types.WriteRequest, 0, len(mhs))

	for _, mh := range mhs {
		deleteRequests = append(deleteRequests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{
					fieldMultihash: &types.AttributeValueMemberB{Value: mh},
					fieldValueKey:  &types.AttributeValueMemberS{Value: valueKey},
				},
			},
		})
	}

	// Process all delete requests in batches
	if err := s.batchWriteItems(deleteRequests, s.multihashMapTable); err != nil {
		return fmt.Errorf("failed to batch delete items: %w", err)
	}

	return nil
}

// RemoveProvider implements indexer.Interface.RemoveProvider
func (s *ddbStore) RemoveProvider(ctx context.Context, providerID peer.ID) error {
	// Collect items to delete
	var lastEvaluatedKey map[string]types.AttributeValue
	var deleteRequests []types.WriteRequest

	for {
		// Query the providers table for all items with the given providerID
		result, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.providersTable),
			KeyConditionExpression: aws.String(fieldProviderID + " = :providerID"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":providerID": &types.AttributeValueMemberS{Value: providerID.String()},
			},
			ExclusiveStartKey: lastEvaluatedKey,
		})
		if err != nil {
			return fmt.Errorf("failed to query provider items for deletion: %w", err)
		}

		// Process items in the current page
		for _, item := range result.Items {
			deleteRequests = append(deleteRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						fieldProviderID: item[fieldProviderID],
						fieldContextID:  item[fieldContextID],
					},
				},
			})
		}

		// If there are no more items, we're done
		if result.LastEvaluatedKey == nil {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	// Process all delete requests in batches
	if err := s.batchWriteItems(deleteRequests, s.providersTable); err != nil {
		return fmt.Errorf("failed to batch delete provider items: %w", err)
	}

	return nil
}

// RemoveProviderContext implements indexer.Interface.RemoveProviderContext
func (s *ddbStore) RemoveProviderContext(providerID peer.ID, contextID []byte) error {
	// Create delete item input
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.providersTable),
		Key: map[string]types.AttributeValue{
			fieldProviderID: &types.AttributeValueMemberS{Value: providerID.String()},
			fieldContextID:  &types.AttributeValueMemberB{Value: contextID},
		},
	}

	// Execute the delete operation
	_, err := s.client.DeleteItem(context.TODO(), input)
	if err != nil {
		// If the item doesn't exist, it's not an error
		var notFoundErr *types.ResourceNotFoundException
		if !errors.As(err, &notFoundErr) {
			return fmt.Errorf("failed to delete provider context: %w", err)
		}
	}

	return nil
}

// Size implements indexer.Interface.Size
func (s *ddbStore) Size() (int64, error) {
	return 0, nil
}

// Flush implements indexer.Interface.Flush
func (s *ddbStore) Flush() error {
	return nil
}

// Close implements indexer.Interface.Close
func (s *ddbStore) Close() error {
	return nil
}

// Iter implements indexer.Interface.Iter
func (s *ddbStore) Iter() (indexer.Iterator, error) {
	return nil, errors.New("DynamoDB valuestore doesn't not support iteration")
}

// Stats implements indexer.Interface.Stats (unsupported for now)
func (s *ddbStore) Stats() (*indexer.Stats, error) {
	return nil, indexer.ErrStatsNotSupported
}
