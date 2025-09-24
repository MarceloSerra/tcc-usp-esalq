package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	tableName = os.Getenv("TABLE_NAME")
	client    *ddb.Client
	cold      = true
)

type Event struct {
	BatchID   string `json:"batch_id"`
	SizeBytes int    `json:"size_bytes"`
	Count     int    `json:"count"`
	Label     string `json:"label"`
}

type Summary struct {
	BatchID        string  `json:"batch_id"`
	Lang           string  `json:"lang"`
	Label          string  `json:"label"`
	SizeBytes      int     `json:"size_bytes"`
	Count          int     `json:"count"`
	BatchElapsedMs int64   `json:"batch_elapsed_ms"`
	PerItemAvgMs   float64 `json:"per_item_ms_avg"`
}

func nowMs() int64 { return time.Now().UnixNano() / 1e6 }

func ensureClient(ctx context.Context) error {
	if client != nil {
		return nil
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	client = ddb.NewFromConfig(cfg)
	return nil
}

func processOne(ctx context.Context, e Event) (Summary, error) {
	if e.BatchID == "" {
		e.BatchID = fmt.Sprintf("batch-%d", time.Now().UnixNano())
	}
	if e.SizeBytes <= 0 {
		e.SizeBytes = 1024
	}
	if e.Count <= 0 {
		e.Count = 100
	}
	if e.Label == "" {
		e.Label = fmt.Sprintf("%dBx%d", e.SizeBytes, e.Count)
	}

	// payload sintético
	payload := make([]byte, e.SizeBytes)
	for i := range payload {
		payload[i] = 'x'
	}
	payloadStr := string(payload)

	var totalMs int64
	tBatch0 := nowMs()

	for seq := 0; seq < e.Count; seq++ {
		t0 := nowMs()
		pk := fmt.Sprintf("batch_id#%s", e.BatchID)
		// 🔒 chave determinística + condição -> idempotente
		sk := fmt.Sprintf("seq#%d", seq)

		_, err := client.PutItem(ctx, &ddb.PutItemInput{
			TableName: &tableName,
			Item: map[string]types.AttributeValue{
				"pk":         &types.AttributeValueMemberS{Value: pk},
				"sk":         &types.AttributeValueMemberS{Value: sk},
				"lang":       &types.AttributeValueMemberS{Value: "go"},
				"label":      &types.AttributeValueMemberS{Value: e.Label},
				"size_bytes": &types.AttributeValueMemberN{Value: fmt.Sprint(e.SizeBytes)},
				"seq":        &types.AttributeValueMemberN{Value: fmt.Sprint(seq)},
				"t_start_ms": &types.AttributeValueMemberN{Value: fmt.Sprint(t0)},
				"payload":    &types.AttributeValueMemberS{Value: payloadStr},
				"cold_start": &types.AttributeValueMemberBOOL{Value: cold},
			},
			ConditionExpression: aws.String("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
		})
		if err != nil {
			var cce *types.ConditionalCheckFailedException
			if errors.As(err, &cce) {
				// já inserido antes (redelivery) -> idempotente: ignora
			} else {
				return Summary{}, err
			}
		}
		t1 := nowMs()
		totalMs += (t1 - t0)
	}

	tBatch1 := nowMs()
	cold = false

	return Summary{
		BatchID:        e.BatchID,
		Lang:           "go",
		Label:          e.Label,
		SizeBytes:      e.SizeBytes,
		Count:          e.Count,
		BatchElapsedMs: tBatch1 - tBatch0,
		PerItemAvgMs:   float64(totalMs) / float64(e.Count),
	}, nil
}

// Handler aceita SQS (batch=1 ou >1) e chamada direta (Function URL/Test)
func handler(ctx context.Context, raw json.RawMessage) (any, error) {
	if err := ensureClient(ctx); err != nil {
		return nil, err
	}

	// Tenta SQS primeiro
	var sqs events.SQSEvent
	if err := json.Unmarshal(raw, &sqs); err == nil && len(sqs.Records) > 0 {
		resp := events.SQSEventResponse{}
		for _, r := range sqs.Records {
			var e Event
			if err := json.Unmarshal([]byte(r.Body), &e); err != nil {
				resp.BatchItemFailures = append(resp.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: r.MessageId})
				continue
			}
			if _, err := processOne(ctx, e); err != nil {
				resp.BatchItemFailures = append(resp.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: r.MessageId})
				continue
			}
		}
		return resp, nil
	}

	// Caso contrário, evento direto
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	return processOne(ctx, e)
}

func main() { lambda.Start(handler) }
