package smsproxy

import (
	"sync"

	"gitlab.com/devskiller-tasks/messaging-app-golang/fastsmsing"
)

type batchingClient interface {
	send(message SendMessage, ID MessageID) error
}

func newBatchingClient(
	repository repository,
	client fastsmsing.FastSmsingClient,
	config smsProxyConfig,
	statistics ClientStatistics,
) batchingClient {
	return &simpleBatchingClient{
		repository:     repository,
		client:         client,
		messagesToSend: make([]fastsmsing.Message, 0),
		config:         config,
		statistics:     statistics,
		lock:           sync.RWMutex{},
	}
}

type simpleBatchingClient struct {
	config         smsProxyConfig
	repository     repository
	client         fastsmsing.FastSmsingClient
	statistics     ClientStatistics
	messagesToSend []fastsmsing.Message
	lock           sync.RWMutex
}

func (b *simpleBatchingClient) send(message SendMessage, ID MessageID) error {

	err := b.repository.save(ID)

	if err != nil {
		return err
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.messagesToSend = append(b.messagesToSend, fastsmsing.Message{PhoneNumber: message.PhoneNumber, Message: message.Message, MessageID: ID})
	if len(b.messagesToSend) >= b.config.minimumInBatch {

		go func(messages []fastsmsing.Message) {
			var err error = nil
			for i := 0; i < calculateMaxAttempts(b.config.maxAttempts); i++ {
				err = b.client.Send(messages)
				if err == nil {
					break
				}
			}

			sendStatistics(messages, err, b.config.maxAttempts, b.config.maxAttempts, b.statistics)
		}(b.messagesToSend)

		b.messagesToSend = make([]fastsmsing.Message, 0)
	}

	return nil
}

func calculateMaxAttempts(configMaxAttempts int) int {
	if configMaxAttempts < 1 {
		return 1
	}
	return configMaxAttempts
}

func lastAttemptFailed(currentAttempt int, maxAttempts int, currentAttemptError error) bool {
	return currentAttempt == maxAttempts && currentAttemptError != nil
}

func sendStatistics(messages []fastsmsing.Message, lastErr error, currentAttempt int, maxAttempts int, statistics ClientStatistics) {
	statistics.Send(clientResult{
		messagesBatch:  messages,
		err:            lastErr,
		currentAttempt: currentAttempt,
		maxAttempts:    maxAttempts,
	})
}
