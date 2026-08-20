# 📚 BSM DISPATCH ENGINE - BỘ CÂU HỎI PHẢN BIỆN CHUẨN BỊ PRESENTATION (QA CHEAT SHEET)

> **Mục tiêu:** Giúp bạn vượt qua các câu hỏi "hỏi xoáy đáp xoay" từ Mentor/Technical Review Board bằng cách chứng minh tư duy thiết kế hệ thống sâu sắc, tối ưu hiệu năng và khả năng thực thi thực tế.

---

## 🧮 PHẦN 1: THUẬT TOÁN ĐIỀU PHỐI (MATCHING ALGORITHM)

### ❓ Câu hỏi 1: Tại sao không áp dụng Thuật toán Hungarian (Bipartite Matching) cho toàn thành phố để tối ưu toàn cục, mà lại phải dùng Greedy Single-Assignment kết hợp H3 Grid?

*   **Câu trả lời chuẩn:**
    1.  **Hạn chế của Hungarian:** Thuật toán Hungarian tìm tập ghép tối ưu toàn cục có độ phức tạp tính toán rất lớn: **$O(n^3)$**. Ở quy mô toàn thành phố với $10,000$ đơn hàng và $10,000$ tài xế đồng thời, số phép tính sẽ lên tới $10^{12}$ (1 nghìn tỷ phép tính), gây nghẽn CPU và sập luồng điều phối thời gian thực lập tức.
    2.  **Yêu cầu dữ liệu tĩnh (Static Batching):** Hungarian yêu cầu phải biết trước toàn bộ tập hợp khách hàng và tài xế tại một thời điểm. Trong thực tế, đơn hàng đến liên tục từng mili-giây (dynamic stream). Nếu có đơn mới lại phải tính toán lại từ đầu, điều này không khả thi.
    3.  **Giải pháp tối ưu của BSM:**
        *   **Luồng chính (Normal):** Sử dụng **Greedy Single-Assignment ($O(K \log K)$)** với $K \le 20$ (chỉ chấm điểm cho tối đa 20 tài xế đã được Location Service lọc sẵn). Tốc độ xử lý cực nhanh ($< 10$ micro-giây), đáp ứng gán đơn lập tức (real-time).
        *   **Luồng giờ cao điểm (Peak):** Kích hoạt Hungarian cục bộ **trong phạm vi từng ô H3 độ phân giải cao** (Resolution 8) với cửa sổ gom đơn rất ngắn ($2 \to 3$ giây). Vì số lượng đơn $M$ và xế $N$ trong một ô H3 nhỏ tại một thời điểm rất ít ($M, N \le 10$), độ phức tạp $O(V^3)$ chỉ tốn tối đa $10^3 = 1000$ phép tính (chạy trong vài micro-giây), giải quyết bài toán tối ưu toàn cục cục bộ mà vẫn bảo toàn hiệu năng hệ thống.

---

## ⚡ PHẦN 2: HIỆU NĂNG VÀ NGÔN NGỮ GO (HIGH PERFORMANCE GO)

### ❓ Câu hỏi 2: Em nói thuật toán đạt "Zero Allocation" (0 B/op). Tại sao điều này lại quan trọng trong Matching Engine và em đã tối ưu như thế nào trong Go?

*   **Câu trả lời chuẩn:**
    1.  **Tầm quan trọng:** Luồng điều phối (Matching Loop) là "hot path" chạy liên tục hàng chục ngàn lần mỗi giây. Trong Go, mỗi khi cấp phát bộ nhớ động trên Heap (Heap Allocation), Garbage Collector (GC) sẽ phải làm việc để dọn rác. Khi lượng rác quá lớn, GC sẽ kích hoạt cơ chế "Stop-The-World" gây dừng hệ thống tạm thời (GC pause), làm tăng đột biến độ trễ (latency spikes) và gây nghẽn cuốc xe.
    2.  **Cách tối ưu trong code:**
        *   **Không tạo con trỏ mới vô tội vạ:** Truyền struct dưới dạng Value thay vì Pointer nếu struct nhỏ, tránh biến bị "escape to heap".
        *   **Re-use Slice:** Sử dụng mảng có kích thước cố định hoặc tái sử dụng slice bằng cách truyền slice được cấp phát sẵn từ ngoài vào qua tham số.
        *   **Tránh dùng Reflection/Interface:** Hạn chế sử dụng `interface{}` hoặc ép kiểu động trong vòng lặp chấm điểm để tránh cấp phát bộ nhớ ẩn (implicit allocations).
        *   **Sử dụng `sync.Pool`:** Để tái sử dụng các struct kết quả chấm điểm nặng mà không cần cấp phát lại.
        *   *Kết quả chứng minh qua Go Benchmark: 0 B/op.*

---

## 🏛️ PHẦN 3: KIẾN TRÚC HỆ THỐNG VÀ TÍCH HỢP (SYSTEM ARCHITECTURE)

### ❓ Câu hỏi 3: Tại sao lại tách bộ lọc cứng Loại xe (Vehicle Type) và khoảng cách quét vào Location Service, thay vì thực hiện trong Matching Engine?

*   **Câu trả lời chuẩn:**
    1.  **Tách biệt trách nhiệm (Separation of Concerns):** Việc quản lý vị trí địa lý, cấu trúc bản đồ H3 và tính toán ETA thuộc về miền nghiệp vụ của **Location Service**. Việc của Matching Engine là chấm điểm nghiệp vụ (Rating, AR, Ví, Aging).
    2.  **Tối ưu hóa băng thông và tài nguyên:** Nếu Matching Engine nhận tất cả tài xế rồi mới lọc, chúng ta sẽ phải truyền một lượng dữ liệu cực lớn qua mạng (network payload) từ Location sang Matching.
    3.  **Cắt giảm tính toán:** Location Service sử dụng chỉ mục không gian (Spatial Index) để loại ngay các tài xế bận/sai loại xe từ tầng DB/Redis Cache, giới hạn danh sách ứng viên gửi sang Matching Engine tối đa chỉ **20 người**. Điều này giúp giảm thiểu $99.95\%$ khối lượng công việc chấm điểm nghiệp vụ không cần thiết.

### ❓ Câu hỏi 4: Khi gán đơn cho tài xế, tại sao Dispatch Service lại chọn khóa lạc quan (Optimistic Lock - `version`) thay vì khóa bi quan (Pessimistic Lock - `SELECT FOR UPDATE`)?

*   **Câu trả lời chuẩn:**
    1.  **Pessimistic Locking (`SELECT FOR UPDATE`)** sẽ khóa dòng dữ liệu trong DB lại, ngăn cản tất cả các tiến trình khác đọc/ghi vào dòng đó cho đến khi transaction kết thúc. Điều này làm giảm đáng kể throughput (băng thông xử lý) của hệ thống khi có nhiều tài xế cùng phản hồi cho một cuốc xe.
    2.  **Optimistic Locking** cho phép mọi tiến trình đọc dữ liệu đồng thời mà không gây khóa. Khi cập nhật trạng thái đơn gán xe, hệ thống so sánh số `version`:
        `UPDATE bookings SET driver_id = ?, status = 'ASSIGNING', version = version + 1 WHERE id = ? AND version = ?`
        Nếu có xung đột gán trùng, chỉ duy nhất 1 truy vấn cập nhật thành công (trả về 1 dòng bị ảnh hưởng), các truy vấn sau sẽ trả về 0 dòng bị ảnh hưởng và tự động thất bại an toàn mà không làm nghẽn DB.

---

## 🔄 PHẦN 4: THÀNH PHẦN HỖ TRỢ (SUPPORT SERVICES)

### ❓ Câu hỏi 5: Dịch vụ Account Service của em quản lý thông tin ví tiền và rating của tài xế. Làm sao để đảm bảo Matching Engine lấy dữ liệu này với độ trễ < 1ms mà không làm quá tải Postgres DB?

*   **Câu trả lời chuẩn:**
    1.  **Thiết kế In-memory Cache:** Account Service sử dụng một cấu trúc dữ liệu lưu trữ trực tiếp trên RAM (như Redis hoặc In-memory map có cơ chế đồng luồng `sync.Map`).
    2.  **Cơ chế đồng bộ dữ liệu:** 
        *   Các trường dữ liệu thay đổi chậm như Rating, Cấp độ tài xế sẽ được ghi xuống Postgres DB trước, sau đó đồng bộ (Sync) lên Cache RAM định kỳ hoặc cập nhật ngay khi có đánh giá mới.
        *   Trường Số dư ví (`wallet_balance`) thay đổi liên tục khi tài xế nạp/rút/trừ tiền hoa hồng. Khi có giao dịch, Account Service sẽ ghi nhận trực tiếp vào Cache RAM trước để phục vụ thuật toán tức thì, sau đó sử dụng hàng đợi tin nhắn (Message Queue) hoặc Outbox Pattern để cập nhật bất đồng bộ xuống Postgres DB một cách an toàn.

### ❓ Câu hỏi 6: Notification Service quản lý kết nối WebSocket của hàng vạn tài xế đồng thời. Em xử lý vấn đề quá tải bộ nhớ và nghẽn luồng (blocking) như thế nào khi gửi tin nhắn hàng loạt?

*   **Câu trả lời chuẩn:**
    1.  **Mô hình Hub-and-Spoke:** Notification Service tổ chức một bộ quản lý tập trung (Websocket Hub). Mỗi kết nối của tài xế chạy trên một Go-routine riêng biệt giao tiếp qua các **Channel**.
    2.  **Tránh nghẽn luồng (Non-blocking Send):** Khi đẩy tin nhắn mời cuốc xe xuống tài xế, hệ thống sử dụng cơ chế ghi không chặn (Non-blocking channel write):
        ```go
        select {
        case client.sendChannel <- message:
        default:
            // Nếu channel bị đầy (do mạng tài xế yếu), tự động ngắt kết nối hoặc bỏ qua để tránh nghẽn luồng chính
            close(client.sendChannel)
            hub.unregister <- client
        }
        ```
    3.  **Quản lý bộ nhớ:** Giới hạn kích thước buffer của channel gửi tin nhắn cho mỗi tài xế ở mức rất thấp (ví dụ: buffer = 16 tin nhắn), đảm bảo hàng vạn kết nối đồng thời cũng không chiếm quá nhiều RAM hệ thống.
