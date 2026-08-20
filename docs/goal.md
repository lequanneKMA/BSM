# TIÊU CHÍ HOÀN THÀNH THUẬT TOÁN ĐIỀU PHỐI (GOAL.MD)

Tài liệu này tổng hợp các tiêu chí hoàn thành (Definition of Done) riêng cho cấu phần Thuật toán & Động cơ điều phối (Dispatch Engine) của bạn, trích xuất từ tài liệu thiết kế hệ thống của nhóm.

---

## 🎯 1. TIÊU CHÍ CHỨC NĂNG (FUNCTIONAL GOALS)

Hàm điều phối nâng cao `FindAndRankDriversAdvanced` phải vượt qua các tiêu chí sau:

*   [ ] **Phân lọc cứng (Hard Filters):**
    *   Loại bỏ tài xế có trạng thái khác `IDLE`.
    *   Loại bỏ tài xế sai loại xe yêu cầu (`driver.vehicleType != booking.vehicleType`).
    *   Loại bỏ tài xế nằm trong danh sách đen bị từ chối ở các lượt trước của chính đơn này (`driver.id IN booking.excludedDriverIds`).
    *   Loại bỏ tài xế đang bị khóa tạm thời 1 phút do bỏ qua cuộc (không phản hồi trong 30s) bằng Redis cooldown lock.
    *   Bộ lọc ví tài xế: Loại tài xế có số dư ví dưới $20.000\text{ VND}$ nếu đơn hàng thanh toán bằng tiền mặt (`PaymentMethod == "CASH"`).
*   [ ] **Co giãn bán kính quét động ($R_{\text{search}}$):**
    *   Tính toán bán kính theo số lượt thử: $R_{\text{search}} = \min(\text{InitialRadius} \times \text{RadiusExpansionRate}^{\text{attempt}},\ \text{MaxRadius})$.
*   [ ] **Hạ ngưỡng điểm tối thiểu để khớp ($MinScore$):**
    *   Tính toán điểm chuẩn giảm dần theo từng lượt thử: $MinScore = \text{InitialMinScore} \times \text{ScoreDecayRate}^{\text{attempt}}$.
    *   Loại bỏ các tài xế có tổng điểm dưới ngưỡng $MinScore$ này.
*   [ ] **Tính toán điểm ưu tiên tích hợp (Composite Score):**
    *   **Khoảng cách ($S_{\text{proximity}}$):** Tính khoảng cách bằng công thức GPS (qua ETA thực tế), ưu tiên tài xế gần (Tối đa 100 điểm).
    *   **Đánh giá sao ($S_{\text{rating}}$):** Trích từ dữ liệu Account Svc, thang điểm tối đa $50.0$ cho tài xế $5.0$ sao.
    *   **Tỷ lệ nhận đơn ($S_{\text{acceptance}}$):** Trích từ dữ liệu Account Svc, thang điểm tối đa $30.0$ cho tài xế $100\%$ tỷ lệ nhận đơn.
    *   **Tỷ lệ hoàn thành ($S_{\text{completion}}$):** Trích từ dữ liệu Account Svc, thang điểm tối đa $20.0$ cho tài xế $100\%$ tỷ lệ hoàn thành cuốc.
    *   **Điểm chờ lâu ($S_{\text{aging}}$):** Cộng điểm ưu tiên cho khách chờ lâu trên hệ thống, giới hạn tối đa $MaxAgingBoost$.
    *   **VIP Boost ($S_{\text{vip\_boost}}$):** Cộng điểm cho khách VIP/Platinum ở những lượt thử đầu tiên, suy giảm theo $0.5^{\text{attempt}}$ ở các lượt sau.
*   [ ] **Xử lý Tie-break (Bằng điểm):**
    *   Nếu hai tài xế bằng điểm nhau, ưu tiên tài xế có thời gian di chuyển (ETA) đón khách ngắn nhất.

---

## ⚡ 2. TIÊU CHÍ HIỆU NĂNG & AN TOÀN (NON-FUNCTIONAL GOALS)

*   [ ] **Tốc độ xử lý cực cao (Ultra-Low Latency):**
    *   Thời gian chạy hàm chấm điểm và xếp hạng cho tối đa $1.000$ tài xế phải nhỏ hơn $5.0\text{ ms}$ (Mục tiêu tối ưu: $< 1.0\text{ ms}$).
*   [ ] **Tối ưu hóa tài nguyên RAM (Zero Heap Allocations):**
    *   Hạn chế tối đa cấp phát bộ nhớ động trên Heap. Mục tiêu đạt mức $0\text{ B/op}$ thông qua tái sử dụng slice.
*   [ ] **Thread-Safety (An toàn đa luồng):**
    *   Không xảy ra lỗi Data Race khi truy cập/ghi nhận cấu hình thuật toán trên bộ nhớ RAM.
*   [ ] **Kiểm thử tự động (Unit Test Coverage):**
    *   Đạt tỷ lệ bao phủ kiểm thử (Test Coverage) tối thiểu $90\%$ cho toàn bộ logic chấm điểm và lọc điều kiện.

---

## 📊 3. PHÂN TÍCH ĐỘ KHÓ (DIFFICULTY RATING)

*   **Độ khó tổng thể:** **7/10** (Khá khó đối với thực tập sinh vì yêu cầu tối ưu hiệu năng và kiểm thử nghiêm ngặt).
*   **Chi tiết độ khó:**
    1.  *Phần viết công thức toán & lọc cơ bản:* **3/10** (Dễ, chỉ là các biểu thức số học thông thường).
    2.  *Phần viết Unit Test & Mocking dữ liệu:* **6/10** (Trung bình, yêu cầu hiểu sâu về Interface và cách cô lập môi trường test).
    3.  *Phần tối ưu hóa bộ nhớ (Zero Allocations) & Đa luồng (Thread-Safety):* **8/10** (Khó, đòi hỏi kiến thức sâu về cơ chế quản lý bộ nhớ của Go và các công cụ Benchmark).
