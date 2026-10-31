package idempotency

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// Contra o DynamoDB do LocalStack: TEST_DYNAMODB_ENDPOINT=http://localhost:4566.
func TestDynamoStore_LocalStack(t *testing.T) {
	endpoint := os.Getenv("TEST_DYNAMODB_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_DYNAMODB_ENDPOINT não definido")
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")))
	if err != nil {
		t.Fatal(err)
	}
	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(endpoint) })
	table := "notification-dedup-test"
	_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            aws.String(table),
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("message_id"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("message_id"), KeyType: types.KeyTypeHash}},
		BillingMode:          types.BillingModePayPerRequest,
	})
	var inUse *types.ResourceInUseException
	if err != nil && !errors.As(err, &inUse) {
		t.Fatal(err)
	}

	s := NewDynamoStore(client, table, 0)
	id := uuid.NewString()
	if ok, err := s.Claim(ctx, id); err != nil || !ok {
		t.Fatalf("primeira reserva: %v %v", ok, err)
	}
	if ok, err := s.Claim(ctx, id); err != nil || ok {
		t.Fatalf("reserva duplicada deveria voltar false: %v %v", ok, err)
	}
	if err := s.Release(ctx, id); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Claim(ctx, id); !ok {
		t.Fatal("depois de liberar, a reserva volta a valer")
	}
}

type fakeDynamo struct {
	putErr error
	put    *dynamodb.PutItemInput
}

func (f *fakeDynamo) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.put = in
	return &dynamodb.PutItemOutput{}, f.putErr
}

func (f *fakeDynamo) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return &dynamodb.DeleteItemOutput{}, nil
}

func TestDynamoStore_Unidade(t *testing.T) {
	f := &fakeDynamo{}
	s := NewDynamoStore(f, "t", time.Hour)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }
	if ok, err := s.Claim(context.Background(), "m1"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if v := f.put.Item["expires_at"].(*types.AttributeValueMemberN).Value; v != "1767229200" {
		t.Fatalf("ttl = %s", v)
	}
	if *f.put.ConditionExpression != "attribute_not_exists(message_id)" {
		t.Fatal("escrita precisa ser condicional")
	}
	f.putErr = &types.ConditionalCheckFailedException{}
	if ok, err := s.Claim(context.Background(), "m1"); ok || err != nil {
		t.Fatal("duplicada")
	}
	f.putErr = errors.New("throttling")
	if _, err := s.Claim(context.Background(), "m1"); err == nil {
		t.Fatal("erro de infraestrutura precisa subir")
	}
	if NewDynamoStore(f, "t", 0).ttl != 7*24*time.Hour {
		t.Fatal("ttl padrão")
	}
}
