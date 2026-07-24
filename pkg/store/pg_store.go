package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"bsm/pkg/models"

	_ "github.com/lib/pq"
)

type PGStore struct {
	db *sql.DB
}

type OutboxEvent struct {
	ID            int64           `json:"id"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   string          `json:"aggregateId"`
	EventType     string          `json:"eventType"`
	Payload       json.RawMessage `json:"payload"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"createdAt"`
}

func NewPGStore(connStr string) (*PGStore, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &PGStore{db: db}, nil
}

// ClearOldBookings clears leftover test bookings and outbox events on startup
func (pg *PGStore) ClearOldBookings() error {
	_, err := pg.db.Exec(`DELETE FROM outbox; DELETE FROM bookings;`)
	return err
}

// CreateBookingWithOutbox creates booking and outbox event in 1 DB Transaction
func (pg *PGStore) CreateBookingWithOutbox(b models.Booking) error {
	tx, err := pg.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	// Insert booking
	bookingQuery := `
		INSERT INTO bookings (id, customer_id, customer_lat, customer_lng, dest_lat, dest_lng, status, attempt, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9, $10)
	`
	_, err = tx.Exec(bookingQuery, b.ID, b.CustomerID, b.CustomerPos.Lat, b.CustomerPos.Lng, b.DestinationPos.Lat, b.DestinationPos.Lng, string(b.Status), b.Attempt, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert booking: %w", err)
	}

	// Prepare outbox payload
	payloadBytes, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("failed to marshal outbox payload: %w", err)
	}

	// Insert Outbox event
	outboxQuery := `
		INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		VALUES ('booking', $1, 'booking.created', $2, 'NEW', $3)
	`
	_, err = tx.Exec(outboxQuery, b.ID, payloadBytes, now)
	if err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}

	return tx.Commit()
}

// UpdateBookingStatusDirect updates booking status in Postgres
func (pg *PGStore) UpdateBookingStatusDirect(bookingID, newStatus, driverID, token string) error {
	query := `
		UPDATE bookings
		SET status = $1, driver_id = $2, assignment_token = $3, updated_at = NOW()
		WHERE id = $4
	`
	_, err := pg.db.Exec(query, newStatus, driverID, token, bookingID)
	return err
}

// UpdateBookingStatusAtomic executes optimistic locking update
func (pg *PGStore) UpdateBookingStatusAtomic(bookingID, newStatus, driverID, token string, currentVersion int) (bool, error) {
	query := `
		UPDATE bookings
		SET status = $1, driver_id = $2, assignment_token = $3, version = version + 1, updated_at = NOW()
		WHERE id = $4 AND version = $5
	`
	res, err := pg.db.Exec(query, newStatus, driverID, token, bookingID, currentVersion)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// FetchPendingOutboxEvents fetches NEW events from outbox table
func (pg *PGStore) FetchPendingOutboxEvents(limit int) ([]OutboxEvent, error) {
	query := `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, status, created_at
		FROM outbox
		WHERE status = 'NEW'
		ORDER BY id ASC
		LIMIT $1
	`
	rows, err := pg.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType, &e.Payload, &e.Status, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// MarkOutboxEventSent updates outbox status to SENT
func (pg *PGStore) MarkOutboxEventSent(id int64) error {
	_, err := pg.db.Exec(`UPDATE outbox SET status = 'SENT' WHERE id = $1`, id)
	return err
}

// GetStuckPendingBookings finds bookings pending > 15 seconds
func (pg *PGStore) GetStuckPendingBookings(stuckDuration time.Duration) ([]models.Booking, error) {
	threshold := time.Now().Add(-stuckDuration)
	query := `
		SELECT id, customer_id, customer_lat, customer_lng, dest_lat, dest_lng, status, attempt, created_at, updated_at
		FROM bookings
		WHERE status = 'PENDING' AND created_at <= $1
	`
	rows, err := pg.db.Query(query, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Booking
	for rows.Next() {
		var b models.Booking
		if err := rows.Scan(&b.ID, &b.CustomerID, &b.CustomerPos.Lat, &b.CustomerPos.Lng, &b.DestinationPos.Lat, &b.DestinationPos.Lng, &b.Status, &b.Attempt, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, nil
}
