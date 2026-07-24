package outbox

import (
	"fmt"
	"log"
	"time"

	"bsm/pkg/mq"
	"bsm/pkg/store"
)

type Publisher struct {
	pgStore  *store.PGStore
	mqClient *mq.MQClient
	stopChan chan struct{}
}

func NewPublisher(pg *store.PGStore, mq *mq.MQClient) *Publisher {
	return &Publisher{
		pgStore:  pg,
		mqClient: mq,
		stopChan: make(chan struct{}),
	}
}

func (p *Publisher) Start() {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		log.Println("🔄 [Outbox Worker] Outbox Publisher đang chạy...")
		for {
			select {
			case <-ticker.C:
				p.processOutboxEvents()
			case <-p.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (p *Publisher) processOutboxEvents() {
	events, err := p.pgStore.FetchPendingOutboxEvents(50)
	if err != nil {
		log.Printf("⚠️ [Outbox Worker] Lỗi đọc Outbox table: %v", err)
		return
	}

	for _, event := range events {
		err := p.mqClient.Publish(mq.RoutingKey, event.Payload)
		if err != nil {
			log.Printf("❌ [Outbox Worker] Lỗi publish sự kiện Outbox #%d tới RabbitMQ: %v", event.ID, err)
			continue
		}

		// Mark Sent
		if err := p.pgStore.MarkOutboxEventSent(event.ID); err != nil {
			log.Printf("⚠️ [Outbox Worker] Lỗi cập nhật status Outbox #%d: %v", event.ID, err)
		} else {
			log.Printf(fmt.Sprintf("📦 [Outbox Worker] Đã chuyển Outbox Event #%d (%s) ➔ RabbitMQ Exchange", event.ID, event.AggregateID))
		}
	}
}

func (p *Publisher) Stop() {
	close(p.stopChan)
}
