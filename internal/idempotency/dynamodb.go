// Package idempotency evita e-mail duplicado quando o SQS reentrega a mensagem: antes de enviar,
// a Lambda "reserva" o message_id com uma escrita condicional no DynamoDB.
package idempotency

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoAPI é o subconjunto do cliente usado aqui.
type DynamoAPI interface {
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, in *dynamodb.DeleteItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

// DynamoStore guarda um item por mensagem processada; o TTL (atributo expires_at) limpa a tabela.
type DynamoStore struct {
	client DynamoAPI
	table  string
	ttl    time.Duration
	now    func() time.Time
}

func NewDynamoStore(client DynamoAPI, table string, ttl time.Duration) *DynamoStore {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &DynamoStore{client: client, table: table, ttl: ttl, now: time.Now}
}

// Claim reserva o id. Devolve false se outro processamento já reservou (mensagem duplicada).
func (s *DynamoStore) Claim(ctx context.Context, id string) (bool, error) {
	now := s.now().UTC()
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item: map[string]types.AttributeValue{
			"message_id": &types.AttributeValueMemberS{Value: id},
			"claimed_at": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			"expires_at": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(s.ttl).Unix(), 10)},
		},
		ConditionExpression: aws.String("attribute_not_exists(message_id)"),
	})
	var cond *types.ConditionalCheckFailedException
	if errors.As(err, &cond) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Release desfaz a reserva quando o envio falha, para a próxima tentativa enviar.
func (s *DynamoStore) Release(ctx context.Context, id string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key:       map[string]types.AttributeValue{"message_id": &types.AttributeValueMemberS{Value: id}},
	})
	return err
}
