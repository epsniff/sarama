package sarama

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

const TestBatchSize = 1000

func TestFuncProducing(t *testing.T) {
	config := NewConfig()
	testProducingMessages(t, config)
}

func TestFuncProducingGzip(t *testing.T) {
	config := NewConfig()
	config.Producer.Compression = CompressionGZIP
	testProducingMessages(t, config)
}

func TestFuncProducingSnappy(t *testing.T) {
	config := NewConfig()
	config.Producer.Compression = CompressionSnappy
	testProducingMessages(t, config)
}

func TestFuncProducingNoResponse(t *testing.T) {
	config := NewConfig()
	config.Producer.RequiredAcks = NoResponse
	testProducingMessages(t, config)
}

func TestFuncProducingFlushing(t *testing.T) {
	config := NewConfig()
	config.Producer.Flush.Messages = TestBatchSize / 8
	config.Producer.Flush.Frequency = 250 * time.Millisecond
	testProducingMessages(t, config)
}

func testProducingMessages(t *testing.T, config *Config) {
	checkKafkaAvailability(t)

	config.Producer.Return.Successes = true
	config.Consumer.Return.Errors = true

	client, err := NewClient(kafkaBrokers, config)
	if err != nil {
		t.Fatal(err)
	}

	master, err := NewConsumerFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := master.ConsumePartition("single_partition", 0, OffsetNewest)
	if err != nil {
		t.Fatal(err)
	}

	producer, err := NewAsyncProducerFromClient(client)
	if err != nil {
		t.Fatal(err)
	}

	expectedResponses := TestBatchSize
	for i := 1; i <= TestBatchSize; {
		msg := &ProducerMessage{Topic: "single_partition", Key: nil, Value: StringEncoder(fmt.Sprintf("testing %d", i))}
		select {
		case producer.Input() <- msg:
			i++
		case ret := <-producer.Errors():
			t.Fatal(ret.Err)
		case <-producer.Successes():
			expectedResponses--
		}
	}
	for expectedResponses > 0 {
		select {
		case ret := <-producer.Errors():
			t.Fatal(ret.Err)
		case <-producer.Successes():
			expectedResponses--
		}
	}
	safeClose(t, producer)

	for i := 1; i <= TestBatchSize; i++ {
		select {
		case <-time.After(10 * time.Second):
			t.Fatal("Not received any more events in the last 10 seconds.")

		case err := <-consumer.Errors():
			t.Error(err)

		case message := <-consumer.Messages():
			if string(message.Value) != fmt.Sprintf("testing %d", i) {
				t.Fatalf("Unexpected message with index %d: %s", i, message.Value)
			}
		}

	}
	safeClose(t, consumer)
	safeClose(t, client)
}

func TestFuncMultiPartitionProduce(t *testing.T) {
	checkKafkaAvailability(t)

	config := NewConfig()
	config.ChannelBufferSize = 20
	config.Producer.Flush.Frequency = 50 * time.Millisecond
	config.Producer.Flush.Messages = 200
	config.Producer.Return.Successes = true
	producer, err := NewSyncProducer(kafkaBrokers, config)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(TestBatchSize)

	for i := 1; i <= TestBatchSize; i++ {
		go func(i int) {
			defer wg.Done()
			msg := &ProducerMessage{Topic: "multi_partition", Key: nil, Value: StringEncoder(fmt.Sprintf("hur %d", i))}
			if _, _, err := producer.SendMessage(msg); err != nil {
				t.Error(i, err)
			}
		}(i)
	}

	wg.Wait()
	if err := producer.Close(); err != nil {
		t.Error(err)
	}
}

func TestFuncProducingToInvalidTopic(t *testing.T) {
	checkKafkaAvailability(t)

	producer, err := NewSyncProducer(kafkaBrokers, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := producer.SendMessage(&ProducerMessage{Topic: "in/valid"}); err != ErrUnknownTopicOrPartition {
		t.Error("Expected ErrUnknownTopicOrPartition, found", err)
	}

	if _, _, err := producer.SendMessage(&ProducerMessage{Topic: "in/valid"}); err != ErrUnknownTopicOrPartition {
		t.Error("Expected ErrUnknownTopicOrPartition, found", err)
	}

	safeClose(t, producer)
}
