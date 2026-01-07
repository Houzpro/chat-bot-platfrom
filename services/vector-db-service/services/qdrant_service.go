package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// ...существующий код...

// GetAllDocuments возвращает все документы коллекции для botID
func (s *QdrantService) GetAllDocuments(ctx context.Context, botID string) ([]map[string]interface{}, error) {
	collectionName := s.getCollectionName(botID)
	exists, err := s.collectionsClient.CollectionExists(ctx, &qdrant.CollectionExistsRequest{
		CollectionName: collectionName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to check collection: %w", err)
	}
	if exists.GetResult() == nil || !exists.GetResult().GetExists() {
		return []map[string]interface{}{}, nil
	}
	// Scroll all points (no limit)
	var results []map[string]interface{}
	var nextPage *qdrant.PointId = nil
	for {
		scrollResult, err := s.pointsClient.Scroll(ctx, &qdrant.ScrollPoints{
			CollectionName: collectionName,
			WithPayload: &qdrant.WithPayloadSelector{
				SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
			},
			Offset: nextPage,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scroll: %w", err)
		}
		for _, point := range scrollResult.Result {
			result := map[string]interface{}{
				"id": formatPointID(point.Id),
			}
			if point.Payload != nil {
				for key, value := range point.Payload {
					result[key] = value.GetStringValue()
				}
			}
			results = append(results, result)
		}
		if scrollResult.NextPageOffset == nil {
			break
		}
		nextPage = scrollResult.NextPageOffset
	}
	return results, nil
}

type QdrantService struct {
	conn               *grpc.ClientConn
	collectionsClient  qdrant.CollectionsClient
	pointsClient       qdrant.PointsClient
	embeddingDimension uint64
	scoreThreshold     float32
}

func NewQdrantService(host, port string) (*QdrantService, error) {
	addr := fmt.Sprintf("%s:%s", host, port)

	// Dimension defaults to 384, but can be overridden via QDRANT_COLLECTION_SIZE
	embeddingDim := uint64(384)
	if dimStr := os.Getenv("QDRANT_COLLECTION_SIZE"); dimStr != "" {
		if dim, err := strconv.Atoi(dimStr); err == nil && dim > 0 {
			embeddingDim = uint64(dim)
		}
	}

	// Read score threshold from environment (0 disables threshold)
	scoreThreshold := float32(0.0) // default to no threshold for maximal recall
	if thresholdStr := os.Getenv("RAG_SCORE_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.ParseFloat(thresholdStr, 32); err == nil {
			scoreThreshold = float32(threshold)
		}
	}

	// Optimized gRPC connection with keepalive and connection pooling
	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024), // 100MB
			grpc.MaxCallSendMsgSize(100*1024*1024),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	return &QdrantService{
		conn:               conn,
		collectionsClient:  qdrant.NewCollectionsClient(conn),
		pointsClient:       qdrant.NewPointsClient(conn),
		embeddingDimension: embeddingDim,
		scoreThreshold:     scoreThreshold,
	}, nil
}

// Close closes the gRPC connection
func (s *QdrantService) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// getScoreThreshold returns the configured score threshold
func (s *QdrantService) getScoreThreshold() float32 {
	return s.scoreThreshold
}

// formatPointID normalizes Qdrant point IDs to a string, handling both UUID and numeric IDs.
func formatPointID(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if uuid := id.GetUuid(); uuid != "" {
		return uuid
	}
	return strconv.FormatUint(id.GetNum(), 10)
}

func (s *QdrantService) getCollectionName(botID string) string {
	// Use bot_id instead of client_id for collection naming
	return fmt.Sprintf("bot_%s", botID)
}

func (s *QdrantService) EnsureCollection(ctx context.Context, botID string) error {
	collectionName := s.getCollectionName(botID)
	exists, err := s.collectionsClient.CollectionExists(ctx, &qdrant.CollectionExistsRequest{
		CollectionName: collectionName,
	})
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if exists.GetResult() != nil && exists.GetResult().GetExists() {
		return nil
	}
	_, err = s.collectionsClient.Create(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     s.embeddingDimension,
					Distance: qdrant.Distance_Cosine,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	return nil
}

func (s *QdrantService) AddDocuments(ctx context.Context, botID string, texts []string, embeddings [][]float32, metadata []map[string]string) ([]string, error) {
	if err := s.EnsureCollection(ctx, botID); err != nil {
		return nil, err
	}
	collectionName := s.getCollectionName(botID)
	docIDs := make([]string, len(texts))
	points := make([]*qdrant.PointStruct, len(texts))

	// Process in parallel batches for better performance
	const batchSize = 100
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		// Prepare points for this batch
		for j := i; j < end; j++ {
			docID := uuid.New().String()
			docIDs[j] = docID
			payload := map[string]*qdrant.Value{
				"text": {
					Kind: &qdrant.Value_StringValue{StringValue: texts[j]},
				},
				"bot_id": { // Changed from client_id to bot_id
					Kind: &qdrant.Value_StringValue{StringValue: botID},
				},
				"upload_date": {
					Kind: &qdrant.Value_StringValue{StringValue: time.Now().UTC().Format(time.RFC3339)},
				},
			}
			for key, value := range metadata[j] {
				payload[key] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: value}}
			}
			points[j] = &qdrant.PointStruct{
				Id: &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: docID}},
				Vectors: &qdrant.Vectors{
					VectorsOptions: &qdrant.Vectors_Vector{
						Vector: &qdrant.Vector{Data: embeddings[j]},
					},
				},
				Payload: payload,
			}
		}

		// Upsert batch with context
		batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := s.pointsClient.Upsert(batchCtx, &qdrant.UpsertPoints{
			CollectionName: collectionName,
			Points:         points[i:end],
		})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("failed to upsert batch %d-%d: %w", i, end, err)
		}
	}

	return docIDs, nil
}

func (s *QdrantService) SearchDocuments(ctx context.Context, botID string, queryEmbedding []float32, limit uint64) ([]map[string]interface{}, error) {
	collectionName := s.getCollectionName(botID)
	exists, err := s.collectionsClient.CollectionExists(ctx, &qdrant.CollectionExistsRequest{
		CollectionName: collectionName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to check collection: %w", err)
	}
	if exists.GetResult() == nil || !exists.GetResult().GetExists() {
		return []map[string]interface{}{}, nil
	}
	// Optimized search with optional score threshold
	threshold := s.getScoreThreshold()
	var thresholdPtr *float32
	if threshold > 0 {
		thresholdPtr = &threshold
	}
	searchResult, err := s.pointsClient.Search(ctx, &qdrant.SearchPoints{
		CollectionName: collectionName,
		Vector:         queryEmbedding,
		Limit:          limit,
		ScoreThreshold: thresholdPtr,
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	results := make([]map[string]interface{}, 0, len(searchResult.Result))
	for i, point := range searchResult.Result {
		result := map[string]interface{}{
			"id":    formatPointID(point.Id),
			"score": point.Score,
		}
		if point.Payload != nil {
			if text, ok := point.Payload["text"]; ok {
				textValue := text.GetStringValue()
				result["text"] = textValue
				// Log first 100 chars of each result with score
				preview := textValue
				if len(preview) > 100 {
					preview = preview[:100]
				}
				log.Printf("[VectorDB] Result %d: score=%.4f, preview=%s...", i+1, point.Score, preview)
			}
			for key, value := range point.Payload {
				if key != "text" && key != "bot_id" && key != "upload_date" {
					result[key] = value.GetStringValue()
				}
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *QdrantService) DeleteDocuments(ctx context.Context, botID string) error {
	collectionName := s.getCollectionName(botID)
	_, err := s.collectionsClient.Delete(ctx, &qdrant.DeleteCollection{
		CollectionName: collectionName,
	})
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}
	return nil
}

func (s *QdrantService) GetStats(ctx context.Context, botID string) (int, error) {
	collectionName := s.getCollectionName(botID)
	exists, err := s.collectionsClient.CollectionExists(ctx, &qdrant.CollectionExistsRequest{
		CollectionName: collectionName,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to check collection: %w", err)
	}
	if exists.GetResult() == nil || !exists.GetResult().GetExists() {
		return 0, nil
	}
	info, err := s.collectionsClient.Get(ctx, &qdrant.GetCollectionInfoRequest{
		CollectionName: collectionName,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get collection info: %w", err)
	}
	if info.GetResult() == nil || info.GetResult().PointsCount == nil {
		return 0, nil
	}
	return int(info.GetResult().GetPointsCount()), nil
}

func (s *QdrantService) ListDocuments(ctx context.Context, botID string, limit int) ([]map[string]interface{}, error) {
	collectionName := s.getCollectionName(botID)
	exists, err := s.collectionsClient.CollectionExists(ctx, &qdrant.CollectionExistsRequest{
		CollectionName: collectionName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to check collection: %w", err)
	}
	if exists.GetResult() == nil || !exists.GetResult().GetExists() {
		return []map[string]interface{}{}, nil
	}
	limitPtr := uint32(limit)
	scrollResult, err := s.pointsClient.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collectionName,
		Limit:          &limitPtr,
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scroll: %w", err)
	}
	results := make([]map[string]interface{}, 0, len(scrollResult.Result))
	for _, point := range scrollResult.Result {
		result := map[string]interface{}{
			"id": formatPointID(point.Id),
		}
		if point.Payload != nil {
			for key, value := range point.Payload {
				result[key] = value.GetStringValue()
			}
		}
		results = append(results, result)
	}
	return results, nil
}
