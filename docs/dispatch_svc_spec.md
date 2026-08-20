# Đặc Tả Kỹ Thuật: dispatch-svc (Matching Engine)

Dành cho nhóm phát triển dịch vụ điều phối và khớp đơn (**`dispatch-svc`**).

---

## ⚡ 1. Tổng Quan Nhiệm Vụ (Scope of Responsibility)

`dispatch-svc` chịu trách nhiệm chính về mặt nghiệp vụ ghép đơn, quản lý vòng đời matching, chấm điểm và gán đơn hàng cho tài xế phù hợp nhất. Dịch vụ này không xử lý tính toán không gian, nó tiêu thụ dữ liệu thô từ `location-svc`, `account-svc` và phối hợp các kịch bản khóa ứng viên.

---

## 💾 2. Cơ Chế Khóa Phân Tán & Khóa Phạt (Redis Locks)

Để giải quyết tranh chấp tài xế trong môi trường concurrency cao, `dispatch-svc` vận hành 2 cơ chế khóa trên **Redis**:

### 2.1 Khóa Giữ Chân Tài Xế (Distributed Booking Lock)
Khi chọn được tài xế số 1 vượt qua bộ lọc và có điểm cao nhất, hệ thống thực hiện gán giữ chân tài xế này trong 30 giây để chờ phản hồi từ Driver App:
*   **Redis Command:** `SET booking:{booking_id}:lock {driver_id} NX EX 30`
*   *Ý nghĩa:* Đảm bảo tài xế này không bị hệ thống gán cho bất cứ đơn hàng nào khác chạy song song. Khóa tự giải phóng khi tài xế Chấp nhận/Từ chối hoặc tự hết hạn sau 30s.

### 2.2 Khóa Phạt Trôi Đơn (Redis Cooldown Lock & DB Lockout Sync)
Nếu tài xế để trôi tin nhắn mời cuốc (Timeout 30s không phản hồi), hệ thống tự động áp dụng cơ chế khóa phạt tài xế này trong 1 phút trên cả hai lớp lưu trữ:
1.  **Lớp Dữ Liệu Bền Vững (Database):** `dispatch-svc` gọi API sang `account-svc` (`PATCH /api/v1/drivers/{driver_id}/status`) để cập nhật trạng thái sang `COOLDOWN` hoặc lưu mốc thời gian khóa phạt `lockout_until = now() + 60s` vào bảng tài xế trong PostgreSQL.
2.  **Lớp Cache Tốc Độ Cao (Redis):** `dispatch-svc` thiết lập khóa phạt trong Redis:
    *   **Redis Command:** `SET driver:{driver_id}:cooldown "locked" EX 60`
    *   *Ý nghĩa:* Trong vòng 60 giây tiếp theo, tài xế này sẽ bị loại trực tiếp tại vòng Bộ lọc cứng của tất cả các đơn hàng khác trên hệ thống nhờ truy vấn nhanh qua Redis. Khi hết 60s, Redis tự động giải phóng key và `account-svc` đồng bộ chuyển trạng thái tài xế về lại `IDLE`.

---

## ⚙️ 3. Quy Trình Bộ Lọc Cứng (Hard Filters)

Trước khi đưa ứng viên vào chấm điểm, `dispatch-svc` loại bỏ ngay lập tức các ứng viên không hợp lệ:
1.  **Trạng thái hoạt động:** Trạng thái tài xế phải là `IDLE` (sẵn sàng đón khách).
2.  **Loại phương tiện:** `driver.vehicle_type == booking.vehicle_type`.
3.  **Blacklist theo đơn:** Loại bỏ tài xế nằm trong danh sách đen bị từ chối của chính đơn hàng này (`driver.id` nằm trong `booking.excluded_driver_ids`).
4.  **Tạm khóa hệ thống (1 phút):** Bỏ qua tài xế nếu tồn tại key `driver:{driver_id}:cooldown` trong Redis.
5.  **Số dư tài khoản ví:** Số dư ví tài xế phải đạt mức tối thiểu $\ge 20.000\text{ VND}$ nếu đơn hàng thanh toán bằng tiền mặt (`PaymentMethod == "CASH"`).

---

## 📈 4. Công Thức Chấm Điểm Hỗn Hợp (Composite Scoring Engine)

Điểm tổng hợp cuối cùng của tài xế được tính theo công thức:
$$\text{TotalScore} = (100 \times \text{DistanceScore}) + (50 \times \text{Rating}) + \left(30 \times \frac{\text{AcceptanceRate}}{100.0}\right) + \left(20 \times \frac{\text{CompletionRate}}{100.0}\right) + \text{VIP\_BOOST} + S_{\text{aging}}$$

### 4.1 Chi Tiết Các Điểm Thành Phần

#### A. Điểm Khoảng Cách Định Tuyến ($\text{DistanceScore}$):
Tính dựa trên thời gian di chuyển thực tế (ETA) đón khách do `location-svc` cung cấp:
$$\text{DistanceScore} = \frac{1.0}{0.001 \times \text{ETA}_{\text{seconds}} + 1.0}$$

#### B. Điểm Đánh Giá Sao từ Khách Hàng ($\text{RatingScore}$):
Lấy từ `account-svc` (Thang điểm 1.0 -> 5.0).
$$\text{RatingScore} = 50 \times \text{Rating}$$

#### C. Điểm Tỷ Lệ Nhận Đơn ($\text{AR Score}$):
Lấy từ `account-svc` (Thang điểm 0% -> 100%).
$$\text{AR Score} = 30 \times \frac{\text{AcceptanceRate}}{100.0}$$

#### D. Điểm Tỷ Lệ Hoàn Thành Cuốc ($\text{CoR Score}$):
Lấy từ `account-svc` (Thang điểm 0% -> 100%).
$$\text{CoR Score} = 20 \times \frac{\text{CompletionRate}}{100.0}$$

#### E. Điểm Khách Hàng VIP Boost ($\text{VIP\_BOOST}$):
Ưu tiên gán tài xế chất lượng cao cho khách VIP ở những lượt thử đầu tiên, giảm dần theo lượt thử `attempt`:
$$\text{VIP\_BOOST} = 10.0 \times 0.5^{\text{attempt}}$$
*(Khách hàng Regular có VIP_BOOST = 0.0)*

#### F. Điểm Cộng Chờ Lâu (Priority Queue Aging - $S_{\text{aging}}$):
Tự động tăng điểm ưu tiên khi khách hàng phải xếp hàng lâu trong hệ thống (tối đa 15 điểm):
$$t_{\text{wait}} = \text{Thời gian hiện tại} - \text{Booking.CreatedAt} \text{ (giây)}$$
$$S_{\text{aging}} = \min(t_{\text{wait}} \times 0.1,\ 15.0)$$

---

### 4.2 Cơ Chế Giảm Ngưỡng Khớp Đơn Động ($MinScore$ Decay)
Để đảm bảo các đơn hàng luôn được khớp thành công ở các lượt quét sau, hệ thống tự động hạ dần yêu cầu chất lượng tối thiểu của tài xế theo từng lượt thử gán (`attempt` chạy từ 0 đến 5):
$$MinScore = \text{InitialMinScore} \times \text{ScoreDecayRate}^{\text{attempt}}$$

*   *Tham số đề xuất:* $\text{InitialMinScore} = 300\text{ điểm}$, $\text{ScoreDecayRate} = 0.8$.
*   **Lượt gán 1 (attempt = 0):** Ngưỡng $\text{MinScore} = 300$ điểm.
*   **Lượt gán 2 (attempt = 1):** Ngưỡng $\text{MinScore} = 240$ điểm.
*   **Lượt gán 3 (attempt = 2):** Ngưỡng $\text{MinScore} = 192$ điểm.
*   **Lượt gán 4 (attempt = 3):** Ngưỡng $\text{MinScore} = 153.6$ điểm.
*   **Lượt gán 5 (attempt = 4):** Ngưỡng $\text{MinScore} = 122.9$ điểm.
*   **Lượt gán 6 (attempt = 5):** Ngưỡng $\text{MinScore} = 98.3$ điểm.

*Xử lý Tie-break:* Nếu hai tài xế bằng điểm nhau, ưu tiên tài xế có `ETA` nhỏ nhất.

---

## 🔌 5. API Contracts (Thiết Kế Cổng Giao Tiếp)

### 5.1 API Chấp Nhận Cuốc Xe (Driver Accept)
*   **Endpoint:** `POST /api/v1/dispatch/accept`
*   **Request Payload:**
    ```json
    {
      "booking_id": "bk_99018",
      "driver_id": "drv_10293"
    }
    ```
*   **Response:** `200 OK` (Khớp thành công, hủy lock và cập nhật BUSY).

### 5.2 API Từ Chối Cuốc Xe (Driver Reject)
*   **Endpoint:** `POST /api/v1/dispatch/reject`
*   **Request Payload:**
    ```json
    {
      "booking_id": "bk_99018",
      "driver_id": "drv_10293"
    }
    ```
*   **Response:** `200 OK` (Giải phóng lock đơn hàng và quay lại luồng điều phối).
