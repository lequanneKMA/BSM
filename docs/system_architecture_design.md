# THIẾT KẾ CHI TIẾT CÁC DỊCH VỤ (SERVICE DESIGN DIAGRAM) - BSM

Tài liệu này mô tả sơ đồ kiến trúc các dịch vụ theo dạng Vi dịch vụ hướng sự kiện (Event-Driven Microservices) được tối ưu hóa cho dự án **BSM (Backend System for Mobility)**.

---

## 1. Sơ Đồ Khối Tầng Dịch Vụ (Service Layer Flowchart)

Sơ đồ thể hiện luồng liên kết và giao tiếp giữa các thành phần phần mềm trong kiến trúc vi dịch vụ:

```mermaid
flowchart TD
    %% Tầng 1: Clients
    subgraph L1 ["1. TẦNG THIẾT BỊ CLIENT"]
        CustApp["Customer App\n(Ứng dụng Khách)"]
        DrvApp["Driver App\n(Ứng dụng Tài xế)"]
    end

    %% Tầng 2: Gateway & API Entry
    subgraph L2 ["2. CỔNG API ENTRY & GATEWAY"]
        OrderSvc["order-svc\n(Quản lý Đơn & Outbox)"]
    end

    %% Tầng 3: Event Broker & Coordinator
    subgraph L3 ["3. EVENT BUS & COORDINATION"]
        KafkaBus[("Apache Kafka\n(Topic: booking.events / notification.push)")]
        DispatchSvc["dispatch-svc\n(Matching Engine)"]
        RedisStore[("Redis Distributed Cache\n(Lock xế 30s)")]
    end

    %% Tầng 4: Supporting Services
    subgraph L4 ["4. CÁC DỊCH VỤ HỖ TRỢ"]
        LocSvc["location-svc\n(Driver GPS & Radius Expansion)"]
        MapSvc["map-svc\n(Routing Engine - OSRM)"]
        AccSvc["account-svc\n(Driver Profile & Status)"]
        NotiSvc["notification-svc\n(WebSocket Push Gateway)"]
    end

    %% Tầng 5: Data Persistence
    subgraph L5 ["5. TẦNG CƠ SỞ DỮ LIỆU"]
        PostgresDB[("PostgreSQL\n(Bảng bookings, outbox_events)")]
    end

    %% Luồng liên kết dữ liệu
    CustApp -->|1. Đặt xe HTTP| OrderSvc
    OrderSvc -->|2. Lấy tuyến/ETA sơ bộ| MapSvc
    OrderSvc -->|3. Ghi DB & Outbox| PostgresDB
    
    %% Outbox Relay publishing
    PostgresDB -.->|4. Outbox Relay poll & publish| KafkaBus
    
    KafkaBus -->|5. Consume booking.created| DispatchSvc
    
    %% Dispatch-Svc calling Location
    DispatchSvc -->|6. Lấy xế rảnh kèm attempt| LocSvc
    LocSvc -->|7. Tính ma trận ETA đường bộ| MapSvc
    DispatchSvc -->|8. Lấy Profile xế| AccSvc
    
    DispatchSvc -->|9. Đăng ký Lock 30s| RedisStore
    DispatchSvc -->|10. Đổi status ASSIGNING| OrderSvc
    DispatchSvc -->|11. Đẩy notification.push| KafkaBus
    
    KafkaBus -->|12. Consume push| NotiSvc
    NotiSvc -->|13. Gửi đơn WebSocket| DrvApp
```

---

## 2. Sơ Đồ Tuần Tự Tương Tác Chi Tiết (Sequence Diagram)

Quy trình tuần tự xử lý đơn từ lúc khách đặt xe, điều phối tìm tài xế, quản lý khóa giữ xế qua Redis, và xử lý phản hồi/timeout:

```mermaid
sequenceDiagram
    autonumber
    actor Customer as Khách hàng
    participant Order as order-svc
    participant Postgres as PostgreSQL
    participant Kafka as Kafka Event Bus
    participant Dispatch as dispatch-svc
    participant Location as location-svc
    participant Map as map-svc
    participant Redis as Redis
    participant Noti as notification-svc
    actor Driver as Tài xế

    Customer->>Order: 1. Đặt xe (POST /api/v1/bookings)
    Order->>Postgres: 2. Ghi Booking (PENDING) & outbox_events
    Order-->>Customer: 3. Trả về 201 Created (Status: PENDING)
    
    Note over Order, Postgres: Outbox Relay quét & phát sự kiện
    Order->>Kafka: 4. Publish "booking.created"
    
    Kafka->>Dispatch: 5. Consume "booking.created"
    
    %% Dispatch queries Location Service with attempt
    Dispatch->>Location: 6. Tìm xế lân cận (GET /nearby?attempt=X)
    Note over Location: Tự co giãn bán kính theo attempt
    Location->>Map: 7. Tính ma trận ETA (POST /routes/batch)
    Map-->>Location: 8. Trả về khoảng cách & ETA đường bộ
    Location-->>Dispatch: 9. Trả về danh sách xế rảnh kèm sẵn ETA
    
    Dispatch->>Redis: 10. Khóa tài xế #1 (SETNX lock 30s)
    
    alt Khóa thành công (Lock Acquired)
        Dispatch->>Order: 11. Đổi status đơn -> ASSIGNING
        Dispatch->>Kafka: 12. Publish "notification.push"
        Kafka->>Noti: 13. Consume "notification.push"
        Noti->>Driver: 14. Đẩy đơn WebSocket (Timer 30s)
    end

    alt Tài xế Chấp nhận (Accept)
        Driver->>Dispatch: 15. Chấp nhận (POST /accept)
        Dispatch->>Redis: 16. Giải phóng Lock tài xế #1
        Dispatch->>Order: 17. Đổi status đơn -> ACCEPTED
        Dispatch->>Kafka: 18. Publish "booking.accepted"
        Noti-->>Customer: 19. WebSocket gửi thông tin tài xế
    else Tài xế Từ chối (Reject) / Hết hạn 30s (Timeout)
        alt Từ chối chủ động
            Driver->>Dispatch: 15. Từ chối (POST /reject)
            Dispatch->>Redis: 16. Giải phóng Lock
        else Hết hạn 30s (Timeout)
            Note over Redis: Lock tự động hết hạn (TTL Expired)
            Redis->>Dispatch: 17. Keyspace Notification hết hạn lock
        end
        Dispatch->>Order: 18. Tăng retry_count & cập nhật SEARCHING
        Note over Dispatch: Quay lại bước 6 (gọi Location Service với attempt mới)
    end
```

---

## 3. Đặc Tả Chi Tiết API & Payload Kết Nối Giữa Các Dịch Vụ

### API 1: Lấy danh sách tài xế kèm ETA (location-svc cung cấp)
*   **Endpoint:** `GET /api/v1/locations/nearby`
*   **Query Parameters:**
    *   `lat`: `21.0285` (Tọa độ đón khách)
    *   `lng`: `105.8542`
    *   `attempt`: `0` (Lượt thử hiện tại để co giãn bán kính)
    *   `vehicle_type`: `motorbike` (Loại xe yêu cầu)
*   **Response Payload (JSON):**
    ```json
    {
      "drivers": [
        { 
          "driver_id": "drv_101", 
          "latitude": 21.0290, 
          "longitude": 105.8550,
          "eta": 90.0,
          "distance_meters": 350.0
        },
        { 
          "driver_id": "drv_102", 
          "latitude": 21.0270, 
          "longitude": 105.8530,
          "eta": 180.0,
          "distance_meters": 620.0
        }
      ]
    }
    ```

### API 2: Tính toán khoảng cách & ETA hàng loạt (giao tiếp nội bộ giữa location-svc và map-svc)
*   **Endpoint:** `POST /api/v1/routes/batch`
*   **Request Payload (JSON):**
    ```json
    {
      "origin": { "latitude": 21.0285, "longitude": 105.8542 },
      "destinations": [
        { "driver_id": "drv_101", "latitude": 21.0290, "longitude": 105.8550 },
        { "driver_id": "drv_102", "latitude": 21.0270, "longitude": 105.8530 }
      ]
    }
    ```
*   **Response Payload (JSON):**
    ```json
    {
      "routes": [
        { "driver_id": "drv_101", "distance_meters": 350.0, "duration_seconds": 90.0 },
        { "driver_id": "drv_102", "distance_meters": 620.0, "duration_seconds": 180.0 }
      ]
    }
    ```

### API 3: Lấy thông tin hồ sơ tài xế (account-svc cung cấp)
*   **Endpoint:** `GET /api/v1/drivers/{id}/profile`
*   **Response Payload (JSON):**
    ```json
    {
      "driver_id": "drv_101",
      "name": "Nguyễn Văn A",
      "rating": 4.8,
      "acceptance_rate": 95.0,
      "wallet_balance": 150000.0,
      "status": "IDLE"
    }
    ```

### API 4: Cập nhật trạng thái hoạt động tài xế (account-svc cung cấp)
*   **Endpoint:** `POST /api/v1/drivers/status`
*   **Request Payload (JSON):**
    ```json
    {
      "driver_id": "drv_101",
      "status": "BUSY"
    }
    ```
*   **Response Payload (JSON):**
    ```json
    {
      "success": true
    }
    ```

---

## 4. Đặc Tả JSON Payload Trên Event Bus (Kafka)

### Topic: `booking.events` (Sự kiện booking.created)
```json
{
  "event_id": "evt_booking_cr_12345",
  "event_type": "BOOKING_CREATED",
  "timestamp": "2026-07-29T09:30:00Z",
  "data": {
    "booking_id": "bk_100",
    "customer_id": "cust_888",
    "customer_tier": "VIP",
    "pickup_latitude": 21.0285,
    "pickup_longitude": 105.8542,
    "dropoff_latitude": 21.0350,
    "dropoff_longitude": 105.8600,
    "vehicle_type": "motorbike",
    "payment_method": "CASH"
  }
}
```

### Topic: `notification.push` (Sự kiện mời nhận cuốc xe)
```json
{
  "event_id": "evt_noti_push_99887",
  "event_type": "DRIVER_OFFER_PUSH",
  "timestamp": "2026-07-29T09:30:05Z",
  "data": {
    "booking_id": "bk_100",
    "driver_id": "drv_101",
    "pickup_distance_meters": 350.0,
    "pickup_eta_seconds": 90.0,
    "estimated_fare": 35000.0,
    "timeout_seconds": 30
  }
}
```
