package main

import (
	"context"
	"fmt"
	"os"
	"time"

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

func nowMs() int64 { return time.Now().UnixNano() / int64(time.Millisecond) }

func handler(ctx context.Context, e Event) (Summary, error) {
	if client == nil {
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return Summary{}, err
		}
		client = ddb.NewFromConfig(cfg)
	}

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
		sk := fmt.Sprintf("ts#%d#%d", t0, seq)

		item := map[string]types.AttributeValue{
			"pk":         &types.AttributeValueMemberS{Value: pk},
			"sk":         &types.AttributeValueMemberS{Value: sk},
			"lang":       &types.AttributeValueMemberS{Value: "go"},
			"label":      &types.AttributeValueMemberS{Value: e.Label},
			"size_bytes": &types.AttributeValueMemberN{Value: fmt.Sprint(e.SizeBytes)},
			"seq":        &types.AttributeValueMemberN{Value: fmt.Sprint(seq)},
			"t_start_ms": &types.AttributeValueMemberN{Value: fmt.Sprint(t0)},
			"payload":    &types.AttributeValueMemberS{Value: payloadStr},
			"cold_start": &types.AttributeValueMemberBOOL{Value: cold},
			// Não temos mem_mb/context direto aqui; você pode passar via env fixa se quiser.
		}

		_, err := client.PutItem(ctx, &ddb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      item,
		})
		if err != nil {
			return Summary{}, err
		}
		t1 := nowMs()
		elapsed := t1 - t0
		totalMs += elapsed

		// Update dos tempos finais — opcional, mas manteria simetria com Python se fizesse UpdateItem
		// Para simplificar, deixamos sem UpdateItem aqui (o custo adicional pode enviesar levemente).
	}

	tBatch1 := nowMs()
	cold = false

	s := Summary{
		BatchID:        e.BatchID,
		Lang:           "go",
		Label:          e.Label,
		SizeBytes:      e.SizeBytes,
		Count:          e.Count,
		BatchElapsedMs: tBatch1 - tBatch0,
		PerItemAvgMs:   float64(totalMs) / float64(e.Count),
	}
	return s, nil
}

func main() {
	lambda.Start(handler)
}
