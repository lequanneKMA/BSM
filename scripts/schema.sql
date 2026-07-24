-- PostgreSQL Schema for Enterprise Dispatch System (Outbox Pattern & Optimistic Locking)

CREATE TABLE IF NOT EXISTS bookings (
    id VARCHAR(64) PRIMARY KEY,
    customer_id VARCHAR(64) NOT NULL,
    customer_lat DOUBLE PRECISION NOT NULL,
    customer_lng DOUBLE PRECISION NOT NULL,
    dest_lat DOUBLE PRECISION NOT NULL,
    dest_lng DOUBLE PRECISION NOT NULL,
    driver_id VARCHAR(64),
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    assignment_token VARCHAR(64),
    attempt INT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1,
    excluded_driver_ids TEXT[],
    assignment_expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox (
    id BIGSERIAL PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id VARCHAR(64) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'NEW',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Optimization Indexes
CREATE INDEX IF NOT EXISTS idx_pending_timeout ON bookings (created_at) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_assigning_timeout ON bookings (assignment_expires_at) WHERE status = 'ASSIGNING';
CREATE INDEX IF NOT EXISTS idx_outbox_new ON outbox (id) WHERE status = 'NEW';
