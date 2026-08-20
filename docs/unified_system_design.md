# TÀI LIỆU THIẾT KẾ HỆ THỐNG ĐIỀU PHỐI TOÀN DIỆN (UNIFIED DISPATCH SYSTEM DESIGN)

Tài liệu này tổng hợp toàn diện cấu trúc hệ thống (Architecture), luồng xử lý thời gian thực (End-to-End Sequence), sự phân vai trong nhóm và các vấn đề thảo luận công nghệ của dự án **BSM (Backend System for Mobility)** theo mô hình microservices mới.

---

## 1. TỔNG QUAN KIẾN TRÚC HỆ THỐNG (SYSTEM ARCHITECTURE)

Sơ đồ dưới đây trực quan hóa cấu trúc các thành phần phần mềm và cách dữ liệu di chuyển thời gian thực giữa các vi dịch vụ qua Event Bus (Kafka), RAM Cache, và Database:

```
  [Customer App]
         │ (HTTP POST /api/v1/bookings)
         ▼
    ┌───────────┐         Outbox Relay         ┌───────┐
    │ order-svc │ ───────────────────────────> │ Kafka │
    └─────┬─────┘                              └───┬───┘
          │ (Transactional Commit)                 │ (Consume booking.created)
          ▼                                        ▼
    ┌───────────┐                         ┌──────────────┐
    │ Postgres  │                         │ dispatch-svc │ ───┐ (Redis SETNX lock)
    └───────────┘                         └──────┬───────┘    │
                                                 │            ▼
                        ┌────────────────────────┴───────────────┐
                        │                                        │
                        ▼                                        ▼
                ┌──────────────┐                          ┌─────────────┐
                │ location-svc │ ───┐ (Tính sẵn ETA)      │ account-svc │
                └──────────────┘    │                     │ (Profiles)  │
                        │           ▼                     └─────────────┘
                        │     ┌───────────┐
                        └────>│  map-svc  │
                              │ (Routing) │
                              └───────────┘
                                    │
                                    │ (Kafka notification.push)
                                    ▼
                          ┌──────────────────┐
                          │ notification-svc │ ──> [Driver App] (WS)
                          └──────────────────┘
```

### Chi tiết các dịch vụ thành phần (Service Detailed Responsibilities):

1.  **`order-svc` (Booking Domain):**
    *   **Nhiệm vụ chính:** Quản lý vòng đời đơn đặt xe (Booking Lifecycle).
    *   **Cơ sở dữ liệu:** PostgreSQL (Bảng `bookings` và `outbox_events`).
    *   **API cung cấp:** 
        *   `POST /api/v1/bookings` (Khách đặt xe: kiểm tra `idempotency_key`, ghi Postgres đơn hàng + sự kiện outbox trong cùng Transaction).
        *   `PATCH /api/v1/bookings/{id}/status` (Cập nhật trạng thái đơn hàng).
        *   `GET /api/v1/bookings/{id}` (Lấy chi tiết đơn hàng).
    *   **Background Worker:** `Outbox Relay` (quét outbox publish sự kiện sang Kafka).
    *   **Kafka Events phát đi:** `booking.events` (event: `BOOKING_CREATED`, `BOOKING_ACCEPTED`, `BOOKING_CANCELLED`, `BOOKING_FAILED`).

2.  **`dispatch-svc` (Coordination Domain):**
    *   **Nhiệm vụ chính:** Lõi điều phối (Matching Engine) chấm điểm, xếp hạng và gán đơn cho tài xế, quản lý chu kỳ thử lại (Retry Loop).
    *   **Database/Cache:** Redis (Distributed lock 30s tránh gán trùng tài xế).
    *   **API cung cấp:** `POST /api/v1/dispatch/accept` và `POST /api/v1/dispatch/reject` để nhận phản hồi nhận/từ chối từ tài xế.
    *   **Kafka Events tiêu thụ:** `booking.events` (lắng nghe `BOOKING_CREATED` để chạy matching, và `BOOKING_CANCELLED` để ngắt matching).
    *   **Kafka Events phát đi:** `notification.push` (gửi lệnh push offer cho driver qua notification-svc).

3.  **`location-svc` (Spatial Domain):**
    *   **Nhiệm vụ chính:** Theo dõi tọa độ GPS trực tuyến của tài xế và tìm kiếm tài xế lân cận.
    *   **Database/Cache:** Redis Geo / H3 Grid Index (in-memory).
    *   **API cung cấp:**
        *   `POST /api/v1/locations/update` (Tài xế cập nhật tọa độ GPS).
        *   `GET /api/v1/locations/nearby` (Nhận `lat`, `lng`, `attempt`, `vehicle_type` để tự động mở rộng bán kính quét theo `attempt`, gọi `map-svc` tính ma trận ETA đường bộ và trả về danh sách tài xế đã kèm sẵn ETA/Distance).

4.  **`map-svc` (Geography Domain):**
    *   **Nhiệm vụ chính:** Lõi định tuyến đường bộ tích hợp OSRM.
    *   **API cung cấp:**
        *   `GET /api/v1/routes/eta` (Tính khoảng cách & ETA 2 điểm).
        *   `POST /api/v1/routes/batch` (Tính toán ma trận định tuyến hàng loạt từ 1 điểm đến N tài xế).

5.  **`account-svc` (Driver Profile & State Domain):**
    *   **Nhiệm vụ chính:** Quản lý hồ sơ tài xế và trạng thái hoạt động trực tuyến.
    *   **Database:** PostgreSQL / MySQL.
    *   **API cung cấp:**
        *   `GET /api/v1/drivers/{id}/profile` (Lấy Rating, Tỷ lệ nhận đơn, Tỷ lệ hoàn thành cuốc, Số dư ví).
        *   `PATCH /api/v1/drivers/{id}/status` (Cập nhật trạng thái `IDLE` hoặc `BUSY` của tài xế).

6.  **`notification-svc` (Communication Gateway):**
    *   **Nhiệm vụ chính:** Quản lý kết nối WebSocket realtime tới thiết bị Khách hàng và Tài xế.
    *   **Database/Cache:** In-memory client connection mapping.
    *   **Kafka Events tiêu thụ:** `notification.push` (lệnh push offer), `booking.events` (đồng bộ trạng thái chuyến đi).
    *   **Logic:** Đẩy tin nhắn realtime xuống client WebSocket và nhận tương tác phản hồi chuyển tiếp tới `dispatch-svc`.

---

## 2. LUỒNG XỬ LÝ CHI TIẾT END-TO-END (SEQUENCE WORKFLOW)

Quy trình phối hợp nghiệp vụ giữa các dịch vụ từ khi khách hàng tạo đơn đến khi hoàn thành/từ chối đơn hàng:

1.  **Tạo đơn:** Khách đặt đơn ➔ `order-svc` nhận request, gọi `map-svc` lấy thông tin khoảng cách và ghi nhận booking `PENDING` + chèn outbox event trong PostgreSQL.
2.  **Đẩy sự kiện:** Outbox Relay phát hiện bản ghi mới trong bảng `outbox_events` ➔ Publish sự kiện `booking.created` vào topic `booking.events` của Kafka.
3.  **Điều phối & Tìm xế:** `dispatch-svc` nhận sự kiện từ Kafka ➔ Gọi `location-svc` lấy danh sách xế rảnh lân cận kèm tham số `attempt`.
4.  **Tự động Co giãn & Tính ETA:** `location-svc` tự mở rộng bán kính quét dựa theo `attempt` ➔ Gọi `map-svc` tính toán ma trận ETA đường bộ ➔ Trả về danh sách tối đa 20 tài xế tốt nhất có đầy đủ `eta` và `distance_meters`.
5.  **Xếp hạng & Khóa:** `dispatch-svc` gọi `account-svc` lấy thông tin profile tài xế ➔ Chạy thuật toán xếp hạng tài xế (có VIP boost cho khách VIP) ➔ Chọn ứng viên số 1 và thực hiện khóa giữ xế 30 giây trong Redis bằng lệnh:
    `SETNX booking:{id}:lock {driver_id} EX 30`
6.  **Gửi thông báo:** `dispatch-svc` gọi `order-svc` đổi trạng thái đơn thành `ASSIGNING`, đồng thời publish sự kiện `notification.push` sang Kafka ➔ `notification-svc` consume sự kiện này và đẩy đơn hàng qua WebSocket xuống Driver App.
7.  **Xử lý phản hồi:**
    *   *Tài xế Chấp nhận (Accept):* Driver App gửi API accept tới `dispatch-svc` ➔ Giải phóng lock, gọi `account-svc` cập nhật trạng thái tài xế sang `BUSY`, gọi `order-svc` cập nhật trạng thái booking sang `ACCEPTED` và publish sự kiện `booking.accepted` sang Kafka.
    *   *Tài xế Từ chối (Reject) hoặc Hết hạn 30s (Timeout):* Giải phóng Redis lock. Nếu hết hạn 30s không phản hồi, `dispatch-svc` tạo thêm khóa cooldown tạm thời 1 phút (`driver:{driver_id}:cooldown` với TTL = 60s) trong Redis để khóa tài xế khỏi mọi lượt gán đơn. Cập nhật tăng `retry_count` và lưu tài xế cũ vào blacklist của đơn trong PostgreSQL. Nếu `retry_count <= 5`, tiếp tục chạy lại chu kỳ điều phối (gọi `location-svc` với `attempt = retry_count`). Nếu vượt quá 5 lần thử, đánh dấu đơn là `FAILED` và thông báo cho khách hàng.

---

## 3. BẢNG PHÂN CHIA TRÁCH NHIỆM TRONG NHÓM (RACI MATRIX)

Để phân định rõ ranh giới phát triển của các nhóm trong mô hình microservices:

| Module/Service | Nhiệm vụ kỹ thuật | Ai làm? | Điểm tích hợp (Integration Points) |
| :--- | :--- | :--- | :--- |
| **location-svc** | Quản lý định vị GPS của tài xế thời gian thực, lưu trữ In-memory Spatial cache, tự động mở rộng bán kính dựa theo `attempt`, gọi `map-svc` tính ETA đường bộ và trả về kết quả pre-calculated. | **Team Location** | API: `GET /locations/nearby?lat=...&lng=...&attempt=X&vehicle_type=...` |
| **order-svc** | API đặt đơn, quản lý vòng đời Booking, lưu trữ PostgreSQL, triển khai Outbox pattern & Outbox Relay. | **Team Dispatch** | API: `POST /api/v1/bookings`, `POST /bookings/status` |
| **map-svc** | Routing Engine kết nối OSRM tính khoảng cách đường bộ và ma trận ETA hàng loạt. | **Team Location** | API: `GET /routes/eta`, `POST /routes/batch` |
| **dispatch-svc** | Core matching logic, tính toán điểm composite score, quản lý Redis lock 30s và retry loop (lên tới 5 lần). | **BẠN (Core Engine)** | Kafka: Consume `booking.events`, Publish `notification.push` |
| **account-svc** | Lưu trữ profile tài xế, quản lý trạng thái rảnh/bận (`IDLE`/`BUSY`). | **BẠN (Core Engine)** | API: `GET /drivers/{id}/profile`, `POST /drivers/status` |
| **notification-svc**| WebSocket Hub đồng bộ realtime, đẩy lời mời nhận cuốc và nhận phản hồi từ Driver/Customer. | **BẠN (Core Engine)** | WebSocket: kết nối realtime tới thiết bị |

---

## 4. CÁC ĐIỂM THẢO LUẬN CÔNG NGHỆ CHỐT PHƯƠNG ÁN (DISCUSSION POINTS)

Cả nhóm thống nhất các giải pháp công nghệ như sau:

1.  **Message Broker (Event Bus):** Sử dụng **Kafka** làm hệ thống truyền tin trung tâm. Các service giao tiếp bất đồng bộ qua các topic `booking.events` và `notification.push` để đảm bảo hệ thống không bị nghẽn (non-blocking) và dễ mở rộng.
2.  **Khóa giữ tài xế 30 giây (Concurrency Lock):** Sử dụng **Redis Distributed Lock** (`SETNX` kết hợp thời gian hết hạn TTL 30s). Giải pháp này đảm bảo tính duy nhất khi gán cuốc, ngăn chặn tuyệt đối tình trạng Double-Booking kể cả khi nhiều worker chạy song song.
3.  **Xử lý Timeout phản hồi:** Sử dụng **Redis Keyspace Notifications** (lắng nghe sự kiện Key Expired `__keyevent@0__:expired`) kết hợp với một Scheduler quét dự phòng mỗi 5 giây trong `dispatch-svc`. Khi sự kiện hết hạn được kích hoạt, hệ thống sẽ tự động giải phóng tài xế cũ và kích hoạt retry.
4.  **Bảo toàn dữ liệu (At-least-once Delivery):** Triển khai **Transactional Outbox Pattern** tại `order-svc`. Đơn hàng và sự kiện outbox được ghi đồng thời vào PostgreSQL qua database transaction, đảm bảo sự kiện không bao giờ bị mất nếu ứng dụng bị crash đột ngột.
5.  **Phục hồi sau sự cố (Crash Recovery):** Khi khởi động lại, `dispatch-svc` sẽ quét DB tìm các booking có trạng thái `ASSIGNING` dở dang để khôi phục lại timer đếm ngược chính xác, bảo toàn tính nhất quán của hệ thống.
