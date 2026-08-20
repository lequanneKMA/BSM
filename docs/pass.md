# LỘ TRÌNH HÀNH ĐỘNG ĐỂ ĐƯỢC NHẬN VÀO TEAM BACKEND GO (PASS.MD)

Tài liệu này là checklist hành động chi tiết dành riêng cho bạn (Algorithm Developer) để chứng minh năng lực kỹ thuật vượt trội và giành suất giữ lại (retention) sau kỳ thực tập.

---

## 🎯 TIÊU CHÍ ĐÁNH GIÁ CỦA TECH LEAD / MENTOR
Để giữ lại một Backend Engineer, Mentor không tìm kiếm một "thợ gõ code", họ tìm kiếm:
1. **Tư duy Hiệu năng cao (High Performance):** Viết code tiết kiệm CPU, tiết kiệm RAM (Allocation-free), hiểu sâu về phần cứng/bộ nhớ.
2. **Kỹ năng Kiểm thử chuyên nghiệp (Unit Test & Mocking):** Đảm bảo code chạy đúng và không bao giờ bị sập ở production.
3. **Tư duy Kiến trúc sạch (Clean Architecture & Decoupling):** Thiết kế interface rõ ràng, tách biệt logic nghiệp vụ khỏi database.

---

## 📅 CHECKLIST HÀNH ĐỘNG CHI TIẾT THEO TUẦN

### TUẦN 1 & 2: THIẾT KẾ & ĐỊNH NGHĨA API CONTRACT
*   [ ] **Làm chủ Interface trong Go:**
    *   Học cách định nghĩa Interface để tách thuật toán ra khỏi Location Service và Account Service.
    *   Cấu trúc hàm: FindAndRankDriversAdvanced(booking, driversWithETA, config).
*   [ ] **Định nghĩa API Contract & WebSocket Contract:**
    *   Thống nhất sớm với đội Location về cấu trúc API trả về 20 tài xế kèm sẵn thông số ETA (giây).
    *   Định nghĩa WebSocket protocol (JSON payload) giữa Notification Service và driver client simulator (gửi đơn mời, nhận phản hồi ACCEPT/REJECT).
    *   Thống nhất tham số loại xe (vehicleType) sẽ được đội Location lọc trực tiếp từ tầng DB/Cache.

### TUẦN 3: THIẾT KẾ BỘ NHỚ ĐỆM (CACHE) & WEBSOCKET HUB
*   [ ] **Thiết kế Cache cho Account Service:**
    *   Xây dựng In-memory Cache trong Account Service để lưu trữ thông tin tĩnh của tài xế (Rating, AR, Ví). Tránh việc thuật toán gọi trực tiếp vào Postgres DB làm chậm thời gian điều phối.
*   [ ] **Thiết kế WebSocket Hub cho Notification Service:**
    *   Xây dựng cấu trúc quản lý kết nối WebSocket đồng thời (Concurrency Hub) để quản lý hàng ngàn kết nối driver client trực tuyến (Active connections).

### TUẦN 4: TUẦN GÁNH TEAM - CODE LÕI THUẬT TOÁN & DỊCH VỤ HỖ TRỢ
*   [ ] **Code lõi thuật toán chấm điểm theo ETA:**
    *   Công thức tính điểm khoảng cách theo ETA: Proximity_Score = 50.0 / (0.01 * ETA_seconds + 1.0)
    *   Triển khai công thức giảm điểm tối thiểu theo lượt thử: Min_Score_Threshold = Initial_Min_Score * (Score_Decay_Rate ^ attempt)
    *   Triển khai điểm chờ lâu (Aging Boost): Wait_Time = Current_Time - Booking.CreatedAt (giây), Aging_Score = min(Wait_Time * Priority_Aging_Rate, Max_Aging_Boost)
*   [ ] **Code logic gửi thông báo & Bộ đếm ngược 15s (Notification Timeout):**
    *   Triển khai WebSocket push gửi thông tin cuốc xe kèm Token xác thực đến driver connection được xếp hạng #1.
    *   Viết Timer đếm ngược 15 giây. Nếu quá 15s tài xế không phản hồi, tự động bắn sự kiện TIMEOUT về cho Dispatch Engine để kích hoạt lượt điều phối tiếp theo.
*   [ ] **Viết Table-Driven Unit Tests:**
    *   Tự viết bộ testcase bao phủ 100% các kịch bản:
        1. Đơn CASH: Loại bỏ các tài xế có ví tiền thấp hơn 20.000đ.
        2. Khách VIP: Kiểm thử cơ chế VIP Loyalty Boost cộng điểm ưu tiên.
        3. Khách bị trôi đơn lâu: Kiểm thử điểm Aging Boost kéo tài xế ở xa lại gần đơn hàng.
*   [ ] **Tự viết Mocking:**
    *   Tạo mockup danh sách 20 tài xế đầu vào với các thông số ETA khác nhau để test độc lập thuật toán mà không cần bật Database/OSRM thật.

### TUẦN 5: TÍCH HỢP TOÀN DIỆN, ĐO ĐẠC HIỆU NĂNG & DEMO
*   [ ] **Tích hợp End-to-End toàn nhóm:**
    *   Khớp nối: API đặt đơn (Dispatch) -> Quét 20 người kèm ETA (Location) -> Chấm điểm (Matching Engine của bạn) -> Đẩy tin nhắn và đếm ngược 15s (Notification Service của bạn) -> Ghi nhận trạng thái (Dispatch).
*   [ ] **Đo đạc Benchmark đạt mức Zero Allocations:**
    *   Viết hàm `BenchmarkFindAndRankDriversAdvanced` trong Go.
    *   Tối ưu hóa các struct và thao tác slice để thuật toán chạy đạt mức **0 B/op** (hoàn toàn không cấp phát động trên Heap, tránh sinh rác cho Garbage Collector).
*   [ ] **Chuẩn hóa Structured Logging:**
    *   Sử dụng thư viện `slog` của Go để ghi log thuật toán chuyên nghiệp (in rõ Attempt, Wait Time, MinScore hiện tại, danh sách ID tài xế được xếp hạng).

---

## 💡 CÁCH SHOW HÀNG (DEMO) KHI THUYẾT TRÌNH VỚI MENTOR
Khi thuyết trình buổi cuối, hãy nhấn mạnh 3 điểm này để Mentor thấy bạn ở đẳng cấp khác biệt:

1.  **Dùng Số liệu chứng minh (Metric-Driven):**
    *   *"Thuật toán của em chạy tính điểm cho 20 tài xế chỉ mất dưới X micro-giây và tiêu tốn **0 bytes** bộ nhớ RAM cấp phát mới nhờ tối ưu hóa cấp phát bộ nhớ trong Go."*
2.  **Thiết kế kiến trúc phân rã dịch vụ xuất sắc:**
    *   *"Hệ thống được chia thành các dịch vụ độc lập: Account Service quản lý hồ sơ và có cache riêng để tối ưu truy vấn, Notification Service quản lý luồng WebSocket và bộ đếm ngược 15s độc lập, giúp tách biệt hoàn toàn khỏi luồng ghi DB của Dispatch Service."*
3.  **Kỹ năng Testing & Độ tin cậy cao:**
    *   *"Toàn bộ giải thuật và cơ chế WebSocket Timeout được bao phủ bởi bộ kiểm thử tự động, đảm bảo cuốc xe không bao giờ bị nghẽn hay rơi vào trạng thái bế tắc (deadlock)."*
