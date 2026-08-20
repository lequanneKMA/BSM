# BÁO CÁO TUẦN 1: THIẾT KẾ KIẾN TRÚC ADVANCED BACKEND DISPATCH ENGINE (BSM)

> **Loại hình dự án:** Backend Engineering Project (Dự án Chuyên sâu Backend cho Nhóm Thực tập)  
> **Trọng tâm kỹ thuật:** **Tập trung xây dựng Core Dispatch Engine chuẩn High-Performance Microservices** — Xử lý Concurrency cao bằng Redis Locks, Event-Driven với Kafka, Transactional Outbox Pattern, và Thuật toán điều phối ưu tiên khách VIP.  
> **Người thực hiện:** Nhóm Thực tập Backend (Go)  
> **Đối tượng báo cáo:** Mentor / Tech Lead  
> **Thời gian:** Tuần 1 — Lên ý tưởng & Thiết kế kiến trúc Backend  

---

## 📚 PHẦN 1: KỸ THUẬT BACKEND NỀN TẢNG CHUYÊN SÂU (EVENT-DRIVEN MICROSERVICES)

Nhóm Backend đã nghiên cứu và thống nhất chuyển đổi từ mô hình Monolith sang **Kiến trúc Vi dịch vụ hướng sự kiện (Event-Driven Microservices Architecture)** nhằm đảm bảo khả năng mở rộng (scalability) và cô lập lỗi (fault isolation):

### 1.1 Sơ đồ Phân tách Khối Dịch vụ (Service Domain Division)
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

### 1.2 Chi tiết 6 Dịch vụ Thành phần (Service Blueprint Specifications)
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

## 🚀 PHẦN 2: CÁC THUẬT TOÁN VÀ CƠ CHẾ ĐIỀU PHỐI ĐƯỢC ÁP DỤNG (DISPATCH ALGORITHMS & MECHANISMS)

Hệ thống điều phối của `dispatch-svc` và `location-svc` áp dụng các giải thuật và cơ chế cốt lõi sau để tối ưu hóa việc khớp đơn và tài xế:

### 2.1 Thuật toán Chấm điểm Hỗn hợp (Composite Scoring Algorithm)
*   **Mục đích:** Xếp hạng các tài xế để chọn ra ứng viên tốt nhất.
*   **Cơ chế:** Tính điểm tổng hợp dựa trên các yếu tố trọng số (ETA, Rating, Tỷ lệ nhận đơn, Tỷ lệ hoàn thành cuốc, VIP Boost, Aging Boost):
    $$\text{TotalScore} = (100 \times \text{DistanceScore}) + (50 \times \text{Rating}) + \left(30 \times \frac{\text{AcceptanceRate}}{100.0}\right) + \left(20 \times \frac{\text{CompletionRate}}{100.0}\right) + \text{VIP\_BOOST} + S_{\text{aging}}$$
    Trong đó:
    *   $\text{DistanceScore} = \frac{1.0}{0.001 \times \text{ETA}_{\text{seconds}} + 1.0}$ (ETA lấy từ `location-svc` tính sẵn).
    *   $\text{Rating}$, $\text{AcceptanceRate}$, và $\text{CompletionRate}$ lấy từ hồ sơ tài xế ở `account-svc`.

### 2.2 Cơ chế Ưu tiên Khách VIP (VIP Boost Priority)
*   **Mục đích:** Đảm bảo khách hàng hạng VIP/Platinum luôn được ghép với những tài xế tốt nhất ở các lượt gán đầu tiên.
*   **Cơ chế:** Cộng thêm điểm ưu tiên cực lớn ở lượt gán đầu và giảm dần theo số lượt thử (`attempt`) để tránh đơn bị treo:
    $$\text{VIP\_BOOST} = 10.0 \times 0.5^{\text{attempt}}$$
    *(Khách hàng Regular có VIP_BOOST = 0.0)*

### 2.3 Cơ chế Cộng điểm chờ lâu (Priority Queue Aging)
*   **Mục đích:** Tránh tình trạng đơn hàng ở khu vực khó bị tài xế bỏ quên quá lâu.
*   **Cơ chế:** Tự động cộng thêm điểm ưu tiên dựa trên thời gian khách hàng xếp hàng chờ trong Queue hệ thống:
    $$S_{\text{aging}} = \min(t_{\text{wait}} \times \text{PriorityAgingRate},\ \text{MaxAgingBoost})$$

### 2.4 Cơ chế Co giãn bán kính tìm kiếm (Dynamic Search Radius Expansion)
*   **Mục đích:** Tự động mở rộng phạm vi tìm kiếm khi không có tài xế nào xung quanh nhận cuốc.
*   **Cơ chế:** Được thực thi ngầm bởi `location-svc`. Khi `dispatch-svc` tăng lượt thử (`attempt`), `location-svc` sẽ tự động nhân bán kính quét ban đầu với hệ số mở rộng để tìm thêm tài xế ở vùng xa hơn:
    $$\text{Radius} = \min(\text{InitialRadius} \times \text{RadiusExpansionRate}^{\text{attempt}},\ \text{MaxRadius})$$

### 2.5 Cơ chế Loại trừ Tài xế Đã Từ Chối & Khóa Tạm Thời (Exclusion List & Redis Cooldown Lock)
*   **Mục đích:** Ngăn việc gửi đơn trùng lặp liên tục cho tài xế từ chối và tạm khóa các tài xế không phản hồi tích cực.
*   **Cơ chế:** 
    *   *Blacklist theo đơn:* Khi tài xế bấm từ chối (`REJECT`), ID của tài xế được lưu vào danh sách `excluded_driver_ids` của đơn hàng đó để bỏ qua ở lượt gán sau.
    *   *Khóa tạm thời hệ thống (1 phút):* Nếu tài xế để hết hạn 30s không phản hồi (Timeout), `dispatch-svc` sẽ tạo một khóa cooldown tạm thời trên Redis `driver:{driver_id}:cooldown` với thời gian sống (TTL) là 60 giây (1 phút). Trong suốt thời gian 1 phút này, tài xế sẽ bị loại bỏ khỏi mọi lượt điều phối của tất cả các đơn hàng trên hệ thống.

## 🎯 PHẦN 3: SCOPE & YÊU CẦU CHUYÊN SÂU BACKEND (SYSTEM SCOPE)

### 3.1 Định vị Mục tiêu Backend (Core Focus)
Hệ thống tập trung tối đa vào tính ổn định, tốc độ phản hồi và tính chính xác của lõi điều phối:

*   **Thời gian phản hồi siêu tốc:** Chạy chấm điểm và xếp hạng cho tối đa 20 ứng viên tốt nhất (đã có sẵn ETA) nhận được từ `location-svc` phải nhỏ hơn 2ms.
*   **Concurrency Guard:** Sử dụng Redis Lock (`SETNX` với TTL 30s) chống việc 2 đơn hàng gán trùng cho 1 tài xế, kết hợp cột `version` (Optimistic Locking) trên PostgreSQL.
*   **Tin cậy dữ liệu:** Giao tiếp hướng sự kiện qua Kafka và đảm bảo không mất sự kiện nhờ Transactional Outbox Pattern tại `order-svc`.

### 3.2 Phân Định Phạm Vi Chi Tiết (In-Scope vs Out-of-Scope)

| Component | IN-SCOPE (Nằm trong hệ thống) | OUT-OF-SCOPE (Loại trừ) |
| :--- | :--- | :--- |
| **State Machine** | `PENDING` ➔ `SEARCHING` ➔ `ASSIGNING` ➔ `ACCEPTED` / `REJECTED` / `FAILED`. | Không viết app Client Mobile thật (chỉ giả lập WebSockets). |
| **Matching Logic** | Chạy chấm điểm trong `dispatch-svc`, khóa tài xế 30 giây bằng Redis, tối đa 5 lần thử lại (`retry_count <= 5`). | Không tích hợp Google Maps API trả phí (dùng OSRM qua map-svc). |
| **Event Bus** | Pub/Sub qua Kafka các chủ đề `booking.events` và `notification.push`. | Không làm cổng thanh toán ngân hàng thực tế. |
| **realtime Hub** | WebSocket Hub kết nối driver simulator gửi/nhận offer. | Không làm Auth / JWT phức tạp (dùng Dummy IDs). |

---

## 🧠 PHẦN 4: KIẾN TRÚC CHI TIẾT CỦA CORE DISPATCH ENGINE (BACKEND PIPELINE)

Quy trình điều phối được kích hoạt tự động theo **Pipeline 5 Giai đoạn**:

### 4.1 Giai đoạn 1: Booking Creation (Khởi tạo đơn hàng)
*   Khách hàng đặt xe thông qua `order-svc` (`POST /api/v1/bookings`).
*   `order-svc` gọi `map-svc` tính ETA cơ bản rồi commit vào Postgres trong cùng một transaction: lưu booking (`PENDING`) và chèn sự kiện `booking.created` vào bảng `outbox_events`.
*   Outbox Relay quét và publish sự kiện sang Kafka.

### 4.2 Giai đoạn 2: Candidate Discovery & ETA Calculation (Tìm kiếm & Tính ETA)
*   `dispatch-svc` consume sự kiện `booking.created` từ Kafka.
*   Gửi request sang `location-svc` (`GET /api/v1/locations/nearby?lat=...&lng=...&attempt=X&vehicle_type=...`).
*   `location-svc` tự động mở rộng bán kính quét dựa trên tham số `attempt` và lọc các tài xế chạy đúng `vehicle_type`.
*   `location-svc` gọi `map-svc` (`POST /api/v1/routes/batch`) để tính toán ETA đường bộ hàng loạt cho các tài xế này rồi trả về danh sách tối đa 20 tài xế đã kèm sẵn `eta` và `distance_meters`.

### 4.3 Giai đoạn 3: Scoring & Ranking (Chấm điểm & Xếp hạng)
*   `dispatch-svc` gọi `account-svc` lấy thông tin Rating, Tỷ lệ nhận đơn và Ví tiền của các ứng viên nhận được.
*   Chạy thuật toán chấm điểm xếp hạng tài xế dựa trên công thức:
    $$\text{Total Score} = 100 \times \text{DistanceScore} + 50 \times \text{Rating} + 30 \times \text{AcceptRate} + \text{VIP\_BOOST}$$
    *Trong đó $\text{VIP\_BOOST} = 10.0 \times 0.5^{\text{attempt}}$ nếu hành khách là VIP.*

### 4.4 Giai đoạn 4: Offer Locking (Khóa giữ tài xế)
*   Chọn tài xế có điểm số cao nhất. Tiến hành khóa tài xế này trong Redis để tránh bị gán trùng cuốc khác:
    `SETNX booking:{booking_id}:lock {driver_id} EX 30`
*   Nếu Lock thành công, `dispatch-svc` gọi `order-svc` để cập nhật trạng thái đơn thành `ASSIGNING`.
*   `dispatch-svc` gửi sự kiện `notification.push` đến Kafka ➔ `notification-svc` consume và đẩy đơn mời nhận cuốc qua WebSocket xuống Driver App.

### 4.5 Giai đoạn 5: Timeout & Retry Loop (Xử lý quá hạn & Thử lại)
*   Đồng hồ đếm ngược 30 giây được khởi tạo.
*   **Chấp nhận (Accept):** Đơn đổi sang `ACCEPTED`, tài xế đổi trạng thái sang `BUSY` ở `account-svc`.
*   **Từ chối (Reject) / Quá hạn (Timeout):** Giải phóng khóa trong Redis. `dispatch-svc` cập nhật tăng `retry_count` trong PostgreSQL.
    *   Nếu `retry_count <= 5`, lặp lại Giai đoạn 2 (truyền `attempt = retry_count` sang `location-svc` để co giãn bán kính).
    *   Nếu `retry_count > 5`, đánh dấu đơn là `FAILED`, thông báo cho khách hàng không tìm thấy tài xế.

---

## 📊 PHẦN 5: MA TRẬN 35 TRƯỜNG HỢP XỬ LÝ CONCURRENCY & EDGE CASES (USE CASES MATRIX)

### 🔴 Nhóm 1: Tạo đơn & Khởi tạo (CS-01 ➔ CS-06)
*   **CS-01:** Tạo đơn hợp lệ ➔ Ghi DB, phát sự kiện `booking.created` vào bảng `outbox_events` ở `order-svc`.
*   **CS-02:** Vùng trống tài xế ➔ `dispatch-svc` quét rỗng từ `location-svc`, đưa đơn về hàng đợi đợi chu kỳ quét sau.
*   **CS-03:** Không có loại xe phù hợp ➔ Loại bỏ tại bước lọc ứng viên của `location-svc`, giữ đơn trạng thái `PENDING`.
*   **CS-04:** Tọa độ đón trùng tọa độ trả ➔ Validate API trả lỗi `400 Bad Request` ngay tại gateway.
*   **CS-05:** Khách VIP ➔ Hàng đợi `bookings` tại `order-svc` ưu tiên đẩy đơn lên xử lý trước tiên (`ORDER BY customer_tier DESC`), đồng thời cộng điểm `VIP Boost` ở bước xếp hạng tài xế.
*   **CS-06:** Khách bấm tạo 2 đơn trong 50ms ➔ Cơ chế khóa phân tán Redis check active booking chặn tạo đơn trùng.

### 🟡 Nhóm 2: Matching & Parallel Scoring (CS-07 ➔ CS-12)
*   **CS-07:** Ghép tài xế có Composite Score cao nhất.
*   **CS-08:** Tài xế ở xa nhưng rating cực cao ➔ Công thức tính điểm cân bằng lại bằng cách áp hệ số trọng số tương ứng.
*   **CS-09:** Retry lần kế tiếp ➔ `dispatch-svc` bỏ qua các tài xế trong blacklist từ chối của đơn hàng.
*   **CS-10:** Tài xế có khoảng cách đón gần nhưng rating trung bình ➔ Thuật toán tự động cân bằng giữa điểm khoảng cách và điểm đánh giá sao theo trọng số tương ứng.
*   **CS-11:** Tài xế âm tiền ví ➔ Bộ lọc cứng loại trừ tài xế khỏi danh sách trước khi tính điểm nếu thanh toán bằng tiền mặt.
*   **CS-12:** 2 Tài xế bằng điểm nhau ➔ Ưu tiên tài xế có thời gian di chuyển (ETA) đón khách ngắn nhất làm giá trị phân định (Tie-breaker).

### 🟢 Nhóm 3: Phản hồi Tài xế & Worker Timeout (CS-13 ➔ CS-18)
*   **CS-13:** Chấp nhận trong 30s ➔ Cập nhật trạng thái `ACCEPTED` ở `order-svc` và `BUSY` ở `account-svc`.
*   **CS-14:** Từ chối trong 30s ➔ Giải phóng Redis Lock, tăng `retry_count`, đẩy đơn quay lại luồng điều phối.
*   **CS-15:** Hết hạn 30s ➔ Redis Lock key của đơn hàng hết hạn (TTL), `dispatch-svc` nhận tín hiệu Keyspace Notification giải phóng đơn, tăng `retry_count`, tự động tạo khóa cooldown lock 1 phút cho tài xế đó trên Redis (`driver:{driver_id}:cooldown` với TTL = 60s) để tạm khóa không phân phối bất cứ đơn nào trong 1 phút, và tiếp tục điều phối.
*   **CS-16:** Tài xế bấm nhận sau 30s ➔ Redis Lock đã mất, API nhận đơn trả lỗi `400 Offer Expired`.
*   **CS-17:** Tài xế từ chối khi khách đã hủy ➔ Trạng thái đơn tại `order-svc` đã là `CANCELLED`, hệ thống giải phóng tài xế về `IDLE`.
*   **CS-18:** Request phản hồi trùng lặp do lag mạng ➔ Xử lý idempotent dựa trên transaction ID / Booking ID.

### 🔵 Nhóm 4: Khách hàng Hủy đơn & State Guard (CS-19 ➔ CS-23)
*   **CS-19:** Hủy khi đơn `PENDING` ➔ Đổi trạng thái sang `CANCELLED` trong `order-svc`, ngắt tiến trình matching.
*   **CS-20:** Hủy khi đơn đang `ASSIGNING` ➔ Đổi sang `CANCELLED`, giải phóng Redis Lock của tài xế đang được gán.
*   **CS-21:** Hủy khi đã `ACCEPTED` ➔ Đổi sang `CANCELLED`, giải phóng tài xế từ `BUSY` về `IDLE`.
*   **CS-22:** Hủy khi đơn đã hoàn thành (`COMPLETED`) ➔ Trả lỗi `400 Bad Request` do trạng thái không hợp lệ.
*   **CS-23:** Khách hủy đơn 1 và đặt đơn 2 trong 100ms ➔ Đơn 1 chuyển `CANCELLED`, đơn 2 tạo mới `PENDING`.

### 🟣 Nhóm 5: Retry & Hết lượt thử (CS-24 ➔ CS-27)
*   **CS-24:** Tìm được tài xế ở lượt retry thứ 2.
*   **CS-25:** Tìm được tài xế ở lượt retry thứ 5.
*   **CS-26:** Vượt quá 5 lượt thử (`retry_count > 5`) ➔ `dispatch-svc` cập nhật trạng thái đơn thành `FAILED` ở `order-svc`, gửi thông báo cho khách.
*   **CS-27:** Hết tổng thời gian chờ (mặc định 3 phút) ➔ Tự động đóng đơn và hủy.

### 🟠 Nhóm 6: Race Conditions & Concurrency Stress Scenarios (CS-28 ➔ CS-32)
*   **CS-28:** 2 đơn gán đồng thời cho 1 tài xế rảnh ➔ Redis `SETNX` đảm bảo chỉ 1 đơn gán thành công, đơn còn lại thất bại khi tạo lock và quay về queue.
*   **CS-29:** Khách hủy đúng lúc tài xế bấm nhận ➔ Optimistic Locking `version` trên `order-svc` bảo đảm chỉ có 1 hành động ghi thành công, hành động còn lại bị rollback.
*   **CS-30:** 2 Worker Timeout cùng quét xử lý quá hạn ➔ Tận dụng Redis Lock bảo đảm chỉ có 1 worker xử lý thành công.
*   **CS-31:** Tài xế vừa kết thúc cuốc và chuyển về trạng thái IDLE đúng lúc matching engine đang chấm điểm ➔ Hệ thống cập nhật trạng thái hoạt động tức thì, đưa tài xế vào tập ứng viên hợp lệ ngay lập tức.
*   **CS-32:** Cập nhật vị trí GPS đồng thời lúc tính toán ➔ Đọc vị trí từ `location-svc` qua HTTP không ảnh hưởng luồng chấm điểm.

### ⚪ Nhóm 7: Hạ tầng Backend & System Failures (CS-33 ➔ CS-35)
*   **CS-33:** Rớt kết nối WebSocket tới tài xế ➔ Hết 30 giây Redis lock tự động giải phóng đơn để tìm người khác.
*   **CS-34:** `dispatch-svc` bị crash giữa chừng ➔ Khi khởi động lại, worker quét các đơn có trạng thái `ASSIGNING` dở dang trên DB để khôi phục hàng đợi điều phối.
*   **CS-35:** Stress test 100 req/s ➔ Đảm bảo độ trễ p99 dưới 20ms cho luồng điều phối nhờ tối ưu bộ nhớ.

---

## 🗄️ PHẦN 6: CHUYÊN SÂU DATABASE ENGINEERING & LỘ TRÌNH THỰC HIỆN

### 6.1 Thiết kế Database PostgreSQL & Optimization Indexes (order-svc)
```sql
-- Bảng Bookings lưu trữ trạng thái đơn hàng (order-svc)
CREATE TABLE IF NOT EXISTS bookings (
    id VARCHAR(64) PRIMARY KEY,
    customer_id VARCHAR(64) NOT NULL,
    driver_id VARCHAR(64),
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    vehicle_type VARCHAR(32) NOT NULL,
    region VARCHAR(64),                         -- [TỪ FIGMA USER] Phục vụ chia phân vùng vận hành/surge pricing
    idempotency_key VARCHAR(255) UNIQUE,       -- [TỪ FIGMA USER] Chống trùng lặp yêu cầu tạo đơn từ client
    cancel_reason VARCHAR(255),                 -- [TỪ FIGMA USER] Ghi nhận lý do hủy đơn
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 5,         -- [TỪ FIGMA USER] Số lần gán xế tối đa (linh hoạt theo phân cấp tier)
    
    -- Tọa độ bắt buộc để location-svc & map-svc tính toán lộ trình & tìm tài xế lân cận
    customer_lat DOUBLE PRECISION NOT NULL,
    customer_lng DOUBLE PRECISION NOT NULL,
    dest_lat DOUBLE PRECISION NOT NULL,
    dest_lng DOUBLE PRECISION NOT NULL,
    
    -- Các trường phục vụ lõi matching và tối ưu đồng thời
    customer_tier VARCHAR(32) NOT NULL DEFAULT 'REGULAR',
    version INT NOT NULL DEFAULT 1,             -- Optimistic Locking chống race conditions
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Bảng Outbox Event Stream (order-svc)
CREATE TABLE IF NOT EXISTS outbox_events (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index tối ưu hóa tốc độ truy vấn của order-svc
CREATE INDEX IF NOT EXISTS idx_bookings_status_tier ON bookings (status, customer_tier) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_outbox_events_pending ON outbox_events (id) WHERE status = 'PENDING';
```

### 6.2 Lộ trình Kỹ thuật 3 Tuần tới
*   **Tuần 2 (Microservices Scaffold & Communication):**
    *   Dựng khung 6 dịch vụ Go (order-svc, dispatch-svc, location-svc, etc.).
    *   Cấu hình các topic `booking.events` và `notification.push` trên Kafka.
    *   Hiện thực hóa Transactional Outbox Pattern & Outbox Relay tại `order-svc`.
*   **Tuần 3 (Core Dispatch Engine, Redis Locks & Realtime):**
    *   Xây dựng API `GET /nearby` tại `location-svc` hỗ trợ co giãn bán kính theo `attempt` và trả kèm ETA/Distance.
    *   Xây dựng thuật toán chấm điểm và xếp hạng tài xế trong `dispatch-svc`.
    *   Tích hợp Redis Lock để quản lý phiên giữ tài xế 30 giây.
*   **Tuần 4 (Testing, Benchmarking & Demo):**
    *   Viết integration test giả lập luồng đầy đủ: Đặt đơn -> Chấm điểm -> Khóa Redis -> Nhận/Từ chối qua WebSocket -> Cập nhật trạng thái.
    *   Stress test tải cao đo đạc latency, kiểm tra rò rỉ bộ nhớ (leak check) và tối ưu hóa zero-allocation cho lõi chấm điểm.
