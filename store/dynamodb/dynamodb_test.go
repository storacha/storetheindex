package dynamodb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ipfs/go-test/random"
	"github.com/ipni/go-indexer-core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDDBStore_Get(t *testing.T) {
	// Create test data
	mh := random.Multihashes(1)[0]
	providerID, err := peer.Decode("12D3KooWQux63rGeKSjnKcAzUXCAsTawD6wo8DtTsPPnvFhoZTE1")
	require.NoError(t, err)

	contextID := []byte("test-context")
	metadata := []byte("test-metadata")

	tests := []struct {
		name          string
		setupMock     func(*MockDynamoDBClient)
		expectedErr   bool
		expectedEmpty bool
		expectedLen   int
	}{
		{
			name: "successful get",
			setupMock: func(mockClient *MockDynamoDBClient) {
				// Mock Query for multihashes table
				mockClient.EXPECT().
					Query(mock.Anything, mock.Anything, mock.Anything).
					Return(&dynamodb.QueryOutput{
						Items: []map[string]types.AttributeValue{
							{
								fieldValueKey: &types.AttributeValueMemberS{
									Value: providerID.String() + "/" + string(contextID),
								},
							},
						},
					}, nil)

				// Mock BatchGetItem for providers table
				mockClient.EXPECT().
					BatchGetItem(mock.Anything, mock.Anything, mock.Anything).
					Return(&dynamodb.BatchGetItemOutput{
						Responses: map[string][]map[string]types.AttributeValue{
							"test-providers": {
								{
									fieldProviderID: &types.AttributeValueMemberS{Value: providerID.String()},
									fieldContextID:  &types.AttributeValueMemberB{Value: contextID},
									fieldMetadata:   &types.AttributeValueMemberB{Value: metadata},
								},
							},
						},
					}, nil)
			},
			expectedErr:   false,
			expectedEmpty: false,
			expectedLen:   1,
		},
		{
			name: "no results from multihashes table",
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					Query(mock.Anything, mock.Anything, mock.Anything).
					Return(&dynamodb.QueryOutput{
						Items: []map[string]types.AttributeValue{},
					}, nil)
			},
			expectedErr:   false,
			expectedEmpty: true,
			expectedLen:   0,
		},
		{
			name: "error querying multihashes table",
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					Query(mock.Anything, mock.Anything, mock.Anything).
					Return(
						(*dynamodb.QueryOutput)(nil),
						assert.AnError,
					)
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockClient := NewMockDynamoDBClient(t)
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			// Create store with mock client
			store := NewStore(mockClient, "test-providers", "test-multihashes")

			// Test Get
			values, exists, err := store.Get(mh)

			// Assertions
			if tt.expectedErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedEmpty, !exists)
			assert.Len(t, values, tt.expectedLen)

			// Additional assertions for successful case
			if tt.expectedLen > 0 {
				assert.Equal(t, providerID, values[0].ProviderID)
				assert.Equal(t, contextID, values[0].ContextID)
				assert.Equal(t, metadata, values[0].MetadataBytes)
			}
		})
	}
}

func TestDDBStore_Put(t *testing.T) {
	// Create test data
	mhs := random.Multihashes(2)
	providerID, err := peer.Decode("12D3KooWQux63rGeKSjnKcAzUXCAsTawD6wo8DtTsPPnvFhoZTE1")
	require.NoError(t, err)
	contextID := []byte("test-context")
	metadata := []byte("test-metadata")

	tests := []struct {
		name        string
		setupMock   func(*MockDynamoDBClient)
		value       indexer.Value
		mhs         []multihash.Multihash
		expectedErr bool
	}{
		{
			name: "successful put with multiple multihashes",
			value: indexer.Value{
				ProviderID:    providerID,
				ContextID:     contextID,
				MetadataBytes: metadata,
			},
			mhs: mhs,
			setupMock: func(mockClient *MockDynamoDBClient) {
				// Expect PutItem for the provider record
				mockClient.EXPECT().
					PutItem(mock.Anything, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
						if *input.TableName != "test-providers" {
							return false
						}
						if v, ok := input.Item[fieldProviderID].(*types.AttributeValueMemberS); !ok || v.Value != providerID.String() {
							return false
						}
						if v, ok := input.Item[fieldContextID].(*types.AttributeValueMemberB); !ok || string(v.Value) != string(contextID) {
							return false
						}
						if v, ok := input.Item[fieldMetadata].(*types.AttributeValueMemberB); !ok || string(v.Value) != string(metadata) {
							return false
						}
						return true
					})).
					Return(&dynamodb.PutItemOutput{}, nil)

				// Expect BatchWriteItem for the multihash mappings
				mockClient.EXPECT().
					BatchWriteItem(mock.Anything, mock.MatchedBy(func(input *dynamodb.BatchWriteItemInput) bool {
						reqs, ok := input.RequestItems["test-multihashes"]
						if !ok || len(reqs) != len(mhs) {
							return false
						}

						expectedValueKey := providerID.String() + "/" + string(contextID)
						for i, req := range reqs {
							if req.PutRequest == nil {
								return false
							}
							item := req.PutRequest.Item
							if v, ok := item[fieldMultihash].(*types.AttributeValueMemberB); !ok || string(v.Value) != string(mhs[i]) {
								return false
							}
							if v, ok := item[fieldValueKey].(*types.AttributeValueMemberS); !ok || v.Value != expectedValueKey {
								return false
							}
						}
						return true
					})).
					Return(&dynamodb.BatchWriteItemOutput{}, nil)
			},
			expectedErr: false,
		},
		{
			name: "empty multihashes still creates provider record",
			value: indexer.Value{
				ProviderID:    providerID,
				ContextID:     contextID,
				MetadataBytes: metadata,
			},
			mhs: []multihash.Multihash{},
			setupMock: func(mockClient *MockDynamoDBClient) {
				// Expect only PutItem for the provider record, no BatchWriteItem
				mockClient.EXPECT().
					PutItem(mock.Anything, mock.Anything).
					Return(&dynamodb.PutItemOutput{}, nil)
			},
			expectedErr: false,
		},
		{
			name: "error during PutItem fails the operation",
			value: indexer.Value{
				ProviderID:    providerID,
				ContextID:     contextID,
				MetadataBytes: metadata,
			},
			mhs: mhs,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					PutItem(mock.Anything, mock.Anything).
					Return(nil, assert.AnError)
			},
			expectedErr: true,
		},
		{
			name: "error during BatchWriteItem fails the operation",
			value: indexer.Value{
				ProviderID:    providerID,
				ContextID:     contextID,
				MetadataBytes: metadata,
			},
			mhs: mhs,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					PutItem(mock.Anything, mock.Anything).
					Return(&dynamodb.PutItemOutput{}, nil)

				mockClient.EXPECT().
					BatchWriteItem(mock.Anything, mock.Anything).
					Return(nil, assert.AnError)
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockClient := NewMockDynamoDBClient(t)
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			// Create store with mock client
			store := NewStore(mockClient, "test-providers", "test-multihashes")

			// Test Put
			err := store.Put(tt.value, tt.mhs...)

			// Assertions
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDDBStore_Remove(t *testing.T) {
	// Create test data
	mhs := random.Multihashes(2)
	providerID, err := peer.Decode("12D3KooWQux63rGeKSjnKcAzUXCAsTawD6wo8DtTsPPnvFhoZTE1")
	require.NoError(t, err)
	contextID := []byte("test-context")

	tests := []struct {
		name        string
		setupMock   func(*MockDynamoDBClient)
		value       indexer.Value
		mhs         []multihash.Multihash
		expectedErr bool
	}{
		{
			name: "successful remove with multiple multihashes",
			value: indexer.Value{
				ProviderID: providerID,
				ContextID:  contextID,
			},
			mhs: mhs,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					BatchWriteItem(mock.Anything, mock.Anything).
					Return(&dynamodb.BatchWriteItemOutput{}, nil)
			},
			expectedErr: false,
		},
		{
			name: "no multihashes provided",
			value: indexer.Value{
				ProviderID: providerID,
				ContextID:  contextID,
			},
			mhs: []multihash.Multihash{},
			setupMock: func(mockClient *MockDynamoDBClient) {
				// No calls should be made to DynamoDB
			},
			expectedErr: false,
		},
		{
			name: "error during batch delete",
			value: indexer.Value{
				ProviderID: providerID,
				ContextID:  contextID,
			},
			mhs: []multihash.Multihash{mhs[0]},
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.On("BatchWriteItem", mock.Anything, mock.Anything).
					Return(nil, assert.AnError)
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockClient := NewMockDynamoDBClient(t)
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			// Create store with mock client
			store := NewStore(mockClient, "test-providers", "test-multihashes")

			// Test Remove
			err := store.Remove(tt.value, tt.mhs...)

			// Assertions
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDDBStore_RemoveProvider(t *testing.T) {
	// Create test data
	providerID, err := peer.Decode("12D3KooWQux63rGeKSjnKcAzUXCAsTawD6wo8DtTsPPnvFhoZTE1")
	require.NoError(t, err)
	contextID1 := []byte("test-context-1")
	contextID2 := []byte("test-context-2")

	tests := []struct {
		name        string
		setupMock   func(*MockDynamoDBClient)
		providerID  peer.ID
		expectedErr bool
	}{
		{
			name:       "successful remove provider with multiple contexts",
			providerID: providerID,
			setupMock: func(mockClient *MockDynamoDBClient) {
				// First query should return two items
				mockClient.EXPECT().
					Query(mock.Anything, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
						if *input.TableName != "test-providers" {
							return false
						}
						if input.KeyConditionExpression == nil {
							return false
						}
						if len(input.ExpressionAttributeNames) != 1 || len(input.ExpressionAttributeValues) != 1 {
							return false
						}
						return true
					})).
					Return(&dynamodb.QueryOutput{
						Items: []map[string]types.AttributeValue{
							{
								fieldProviderID: &types.AttributeValueMemberS{Value: providerID.String()},
								fieldContextID:  &types.AttributeValueMemberB{Value: contextID1},
							},
							{
								fieldProviderID: &types.AttributeValueMemberS{Value: providerID.String()},
								fieldContextID:  &types.AttributeValueMemberB{Value: contextID2},
							},
						},
					}, nil)

				// Expect BatchWriteItem to delete both items
				mockClient.EXPECT().
					BatchWriteItem(mock.Anything, mock.MatchedBy(func(input *dynamodb.BatchWriteItemInput) bool {
						reqs, ok := input.RequestItems["test-providers"]
						if !ok || len(reqs) != 2 {
							return false
						}

						// Check that both context IDs are being deleted
						foundContext1 := false
						foundContext2 := false

						for _, req := range reqs {
							if req.DeleteRequest == nil {
								return false
							}
							key := req.DeleteRequest.Key
							if v, ok := key[fieldProviderID].(*types.AttributeValueMemberS); !ok || v.Value != providerID.String() {
								return false
							}
							if v, ok := key[fieldContextID].(*types.AttributeValueMemberB); ok {
								if string(v.Value) == string(contextID1) {
									foundContext1 = true
								} else if string(v.Value) == string(contextID2) {
									foundContext2 = true
								}
							}
						}

						return foundContext1 && foundContext2
					})).
					Return(&dynamodb.BatchWriteItemOutput{}, nil)
			},
			expectedErr: false,
		},
		{
			name:       "no items to delete",
			providerID: providerID,
			setupMock: func(mockClient *MockDynamoDBClient) {
				// Query returns no items
				mockClient.EXPECT().
					Query(mock.Anything, mock.Anything).
					Return(&dynamodb.QueryOutput{
						Items: []map[string]types.AttributeValue{},
					}, nil)
				// No BatchWriteItem should be called
			},
			expectedErr: false,
		},
		{
			name:       "error during query",
			providerID: providerID,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					Query(mock.Anything, mock.Anything).
					Return(nil, assert.AnError)
			},
			expectedErr: true,
		},
		{
			name:       "error during batch delete",
			providerID: providerID,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					Query(mock.Anything, mock.Anything).
					Return(&dynamodb.QueryOutput{
						Items: []map[string]types.AttributeValue{
							{
								fieldProviderID: &types.AttributeValueMemberS{Value: providerID.String()},
								fieldContextID:  &types.AttributeValueMemberB{Value: contextID1},
							},
						},
					}, nil)

				mockClient.EXPECT().
					BatchWriteItem(mock.Anything, mock.Anything).
					Return(nil, assert.AnError)
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockClient := NewMockDynamoDBClient(t)
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			// Create store with mock client
			store := NewStore(mockClient, "test-providers", "test-multihashes")

			// Test RemoveProvider
			err := store.RemoveProvider(context.Background(), tt.providerID)

			// Assertions
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDDBStore_RemoveProviderContext(t *testing.T) {
	// Create test data
	providerID, err := peer.Decode("12D3KooWQux63rGeKSjnKcAzUXCAsTawD6wo8DtTsPPnvFhoZTE1")
	require.NoError(t, err)
	contextID := []byte("test-context")

	tests := []struct {
		name        string
		setupMock   func(*MockDynamoDBClient)
		providerID  peer.ID
		contextID   []byte
		expectedErr bool
	}{
		{
			name:       "successful remove provider context",
			providerID: providerID,
			contextID:  contextID,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					DeleteItem(mock.Anything, mock.Anything).
					Return(&dynamodb.DeleteItemOutput{}, nil)
			},
			expectedErr: false,
		},
		{
			name:       "item not found",
			providerID: providerID,
			contextID:  contextID,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					DeleteItem(mock.Anything, mock.Anything).
					Return(nil, &types.ResourceNotFoundException{Message: aws.String("not found")})
			},
			expectedErr: false, // Not found is not considered an error
		},
		{
			name:       "dynamodb error",
			providerID: providerID,
			contextID:  contextID,
			setupMock: func(mockClient *MockDynamoDBClient) {
				mockClient.EXPECT().
					DeleteItem(mock.Anything, mock.Anything).
					Return((*dynamodb.DeleteItemOutput)(nil), assert.AnError)
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockClient := NewMockDynamoDBClient(t)
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			// Create store with mock client
			store := NewStore(mockClient, "test-providers", "test-multihashes")

			// Test RemoveProviderContext
			err := store.RemoveProviderContext(tt.providerID, tt.contextID)

			// Assertions
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
