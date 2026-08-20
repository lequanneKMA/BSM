# BSM (Backend System for Mobility) - Driver Scoring Algorithm Catalog

Tài liệu này tổng hợp danh mục tham số (metrics), mô hình thuật toán chấm điểm cốt lõi và kiến trúc phân bổ xe động được lựa chọn cho hệ thống điều phối BSM. Thiết kế tập trung vào tính tối ưu vận hành, nâng cao doanh thu cho doanh nghiệp và khả năng mở rộng chịu tải 100.000+ tài xế online đồng thời.

---

## 1. Danh mục các tham số chiều độ (Metrics Catalog)

Các tham số đầu vào được gom nhóm thành 5 nhóm chiều độ chính (các tham số gắn nhãn **Phase 2** là các tính năng ứng viên tương lai, chưa kích hoạt trong bản v1.0 để tránh gây hiểu nhầm cho bộ phận phát triển):

### 1.1. Chiều độ Không gian & Giao thông (Spatial & Temporal Metrics)
*   **ETA Đường Bộ Thực Tế ($t_{ETA}$):** Thời gian di chuyển ước tính, tính bằng **giây (seconds)** từ tọa độ tài xế tới điểm đón khách, do OSRM API tính toán.
*   **Khoảng Cách Đường Bộ Thực Tế ($d_{road}$):** *(Phase 2 — Chưa active trong v1.0)* Khoảng cách di chuyển đường bộ (mét).
*   **Độ Lệch Hướng Di Chuyển ($\theta_{heading}$):** Truyền dưới dạng tham số `bearing` vào OSRM API ở Tầng 3 để $t_{ETA}$ tự động tính toán thời gian đi vòng/quay đầu, tránh bị phạt trùng (double-counting).
*   **Hệ Số Kẹt Xe / Mật Độ Giao Thông ($\rho_{traffic}$):** *(Phase 2 — Chưa active trong v1.0)* Mức độ ùn tắc giao thông.
*   **Chỉ Số Rào Cản Vật Lý ($B_{barrier}$):** Thang đo số nguyên $B_{barrier} \in [0, 5]$ (0 = không rào cản, 1–2 = nhẹ, 3 = trung bình, 4–5 = nghiêm trọng — hẻm nhỏ không quay đầu, khu đô thị khép kín có bảo vệ, rào chắn tạm thời). Với hệ số $w_{barrier}=0.20$ (BSM Car), sàn $G_{barrier}=0.4$ đạt được chính xác tại $B_{barrier}=3$ và giữ nguyên cho $B_{barrier} > 3$.

### 1.2. Chiều độ Chất lượng & Vận hành của Tài xế (Driver Profile & Quality Metrics)
*   **Đánh Giá Sao ($R_{star}$):** Điểm trung bình đánh giá từ khách hàng (Rating: $1.0 \to 5.0$).
*   **Tỷ Lệ Nhận Đơn ($AR$ - Acceptance Rate):** Biểu diễn dưới dạng phần trăm trên thang đo 0–100: $AR = \dfrac{\text{số đơn nhận}}{\text{số đơn bắn xuống}} \times 100$, tính trong cửa sổ 24h. *Quy tắc loại trừ:* Bỏ qua không tính vào AR đối với các trường hợp từ chối hợp lệ (tài xế đã bấm xin nghỉ ca, sự cố ứng dụng/hệ thống).
*   **Tỷ Lệ Hoàn Thành Cuốc ($CoR$ - Completion Rate):** Biểu diễn dưới dạng phần trăm trên thang đo 0–100: $CoR = \dfrac{\text{số đơn hoàn thành}}{\text{số đơn đã nhận}} \times 100$.
*   **Chỉ Số Uy Tín / Cấp Độ Tài Xế ($L_{tier}$):** *(Phase 2 — Chưa active trong v1.0)* Phân hạng tài xế (Regular, Silver, Gold, Platinum).
*   **Số Đơn Hoàn Thành Tích Lũy ($N_{trips}$):** *(Phase 2 — Chưa active trong v1.0)* Kinh nghiệm chạy xe.
*   **Cảnh Báo Gần Đây ($F_{penalty}$):** Tổng số vi phạm ghi nhận trong 24h, $F_{penalty} = F_{reject} + F_{timeout}$ (định nghĩa chi tiết từng loại vi phạm và cơ chế cooldown tương ứng tại Mục 2.5.B / 3.3). Hiện $F_{penalty}$ chỉ mang tính thống kê/quan sát trong v1.0 và chưa được dùng làm điều kiện lọc cứng.

### 1.3. Thời gian chờ đơn của tài xế (Driver Idle Time Metric)
*   **Thời gian chờ đơn ($t_{idle}$):** Thời gian tài xế đã online ở trạng thái sẵn sàng đón khách (`IDLE`) tính bằng giây. Dùng cho công bằng FIFO.

### 1.4. Chiều độ Cung Cầu & Trạng thái Đơn hàng (System State & Order Metrics)
*   **Thời Gian Đợi Của Đơn Hàng ($t_{wait}$):** Thời gian đơn hàng nằm trong hàng đợi chờ gán xe (tính bằng giây).
*   **Mật Độ Cung Cầu Phân Vùng ($S_{D,ratio}$):** Mật độ Khách/Tài xế rảnh trong ô H3 Grid (EMA Smoothed Ratio).
*   **Hạng Khách Hàng ($C_{vip}$):** Điểm ưu tiên nếu khách hàng thuộc nhóm VIP/Platinum ($C_{vip} \in \{0.0, 10.0\}$ trong v1.0: 0.0 = khách thường, 10.0 = khách VIP/Platinum).
*   **Giá Trị Chuyến Đi ($V_{fare}$):** Giá trị tiền cước ước tính của cuốc xe (VNĐ).

### 1.5. Chiều độ Kinh doanh và Doanh thu Doanh nghiệp (Business & Revenue Metrics)
*   **Xác suất bảo toàn doanh thu chuyến đi ($P_{revenue}$):** Đánh giá dựa trên tỷ lệ hoàn thành cuốc ($CoR$) và hệ số cước $FareRatio$: $P_{revenue} = \min\left(3.0,\ \text{FareRatio} \times \frac{CoR}{100}\right)$.
*   **Biên lợi nhuận ròng ($M_{commission}$):** *(Phase 2 — Chưa active trong v1.0)* Chiết khấu giữ lại.
*   **Chi phí cơ hội nhiên liệu ($C_{fuel}$):** *(Phase 2 — Chưa active trong v1.0)* Hao tổn di chuyển rỗng.
*   **Tính thanh khoản hoa hồng lập tức ($L_{cash}$):** *(Phase 2 — Chưa active trong v1.0)* Số dư ví tài khoản tài xế tại `account-svc`.
*   **Hiệu suất khai thác xe vòng quay ($V_{velocity}$):** *(Phase 2 — Chưa active trong v1.0)* Tốc độ hoàn thành cuốc.
*   **Đóng góp vào tỷ lệ giữ chân khách hàng ($R_{retention}$):** *(Phase 2 — Chưa active trong v1.0)* Lịch sử tương thích Khách - Xế.

---

## 2. Thuật toán & Kiến trúc Điều phối Xe Động (Dynamic Dispatch Algorithm & Scaling Architecture)

### 2.1. Mô hình chấm điểm phi tuyến cốt lõi (Non-linear Reciprocal Decay Core Model)

#### Công thức toán học chuẩn hóa:
$$Score_{total} = \underbrace{\left( \frac{100}{1.0 + \alpha \cdot t_{ETA}} \right) \cdot \left[ w_1 \cdot \left(\frac{R_{star}}{5.0}\right)^2 + w_2 \cdot \left(\frac{AR}{100}\right) + w_3 \cdot \left(\frac{CoR}{100}\right) \right] \cdot G_{barrier}}_{Score_{core} \in [0, 100]} + S_{boost} \qquad (S_{boost} \le 30.0)$$

#### Định nghĩa Tham số Trọng số & Ràng buộc Config Invariant (#1, #7):
*   **Bắt buộc tuân thủ Ràng buộc Config (Invariant):** Hệ thống `dispatch-svc` khi load config phải kiểm tra điều kiện $w_1 + w_2 + w_3 = 1.0$, và kiểm tra tổng giá trị hardcoded tối đa của các thành phần $S_{boost}$ ($10.0 + 10.0 + 7.0 + 3.0 = 30.0$). Nếu sai lệch sẽ ngắt khởi động (Panic).
*   **Cấu hình Trọng số Mặc định (Default Config):**
    *   **Dịch vụ Xe máy (BSM Bike):** $w_1 = 0.40, w_2 = 0.30, w_3 = 0.30$; $\alpha_{ETA} = 0.008$; $w_{barrier} = 0.0 \implies G_{barrier} = 1.0$.
    *   **Dịch vụ Ô tô (BSM Car):** $w_1 = 0.50, w_2 = 0.25, w_3 = 0.25$; $\alpha_{ETA} = 0.003$; $w_{barrier} = 0.20 \implies G_{barrier} = \max(0.4, 1 - 0.20 \cdot B_{barrier})$.
*   **Định nghĩa Hệ số $FareRatio$ (#7):** 
    $$\text{FareRatio} = \min\left(3.0,\ \frac{V_{fare}}{\text{Fare}_{\text{avg\_zone\_hour}}}\right)$$
    *Ví dụ số:* Giá cước trung bình khu vực Quận 1 lúc 8h sáng là 50.000 VNĐ. Cuốc xe đi sân bay có giá $V_{fare} = 150.000$ VNĐ $\implies \text{FareRatio} = 150/50 = 3.0$. Với tài xế có $CoR = 90$ (thang 0–100), điểm $P_{revenue} = \min\left(3.0,\ 3.0 \times \frac{90}{100}\right) = 2.7$ điểm.
*   **Phân định Scope Bộ đếm Lượt thử (`order_attempt` vs `driver_attempt`) (#6, #20):**
    *   `order_attempt`: Số lần đơn hàng bị re-dispatch do tài xế từ chối/timeout. Dùng cho $MinScore$ decay và $S_{VIP}$ decay.
    *   `driver_attempt`: Số lần tài xế bị bắn đơn trong ca làm việc.
*   **Triết lý Suy giảm $S_{VIP}$ theo `order_attempt` (#6):** 
    $$S_{VIP}(\text{order\_attempt}) = C_{vip} \times 0.8^{\text{order\_attempt}}$$
    với $C_{vip} \in \{0.0, 10.0\}$ trong v1.0 ($0.0$ = khách thường, $10.0$ = khách VIP/Platinum).
    *Rationale triết lý vận hành:* Việc giảm nhẹ ưu tiên VIP qua mỗi lượt thử là để **tránh ép boost giả tạo (starvation prevention)**. Nếu giữ nguyên $S_{VIP}=10$ static khi đơn bị nhiều tài xế gần từ chối, hệ thống sẽ liên tục cố chấp bắn đơn cho các tài xế cực kỳ xa, làm tăng ETA vô lý thay vì chấp nhận nới lỏng tiêu chí chọn tài xế gần hơn có điểm số vừa phải. Cấu trúc nhân $C_{vip}$ cho phép mở rộng thành thang điểm nhiều bậc (VIP vs Platinum) trong tương lai mà không đổi công thức decay.
*   **Hàm bão hòa $S_{aging}$ cải tiến (#9):** 
    $$S_{aging} = \min\left(10.0,\ 10.0 \times \left(1 - e^{-0.005 \cdot t_{wait\_seconds}}\right)\right)$$
    *Thay đổi:* Hàm hàm mũ bão hòa mượt giúp $S_{aging}$ phân biệt được đơn chờ 1 phút (+2.6 điểm), 3 phút (+5.9 điểm) và 10 phút (+9.5 điểm), không bị nghẽn bão hòa quá sớm ở 50s như công thức tuyến tính cũ.
*   **Công thức $S_{idle\_fifo}$ chuẩn hóa:**
    $$S_{idle\_fifo} = \min\left(7.0,\ 7.0 \times \left(1 - e^{-\beta \cdot t_{idle}}\right)\right)$$
    với $\beta$ là hằng số làm mịn cần benchmark thực tế (tương tự $S_{aging}$ dùng $0.005$), giá trị đề xuất khởi điểm $\beta = 0.001$.
*   **Tổng trần $S_{boost} \le 30.0$:** Với các cận trên $S_{aging} \le 10.0$, $S_{VIP} \le 10.0$, $S_{idle\_fifo} \le 7.0$, $P_{revenue} \le 3.0$, tổng $S_{boost} = S_{aging} + S_{VIP} + S_{idle\_fifo} + P_{revenue} \le 30.0$ và $Score_{total} \le 130.0$ được đảm bảo đúng như công bố.

---

### 2.2. Phân tầng lọc 3 bước & Latency Waterfall (3-Stage Cascading Pipeline)

Phân chia trách nhiệm xử lý rõ ràng giữa `location-svc` và `dispatch-svc` với Waterfall độ trễ quy định cụ thể (#2):

```mermaid
gantt
    title BSM Dispatch Pipeline Latency Waterfall (P95 SLA)
    dateFormat  SS.SSS
    axisFormat %S.%L ms

    section Location-Svc
    Stage 1 - H3 Spatial Filter      :a1, 00.000, 1.5ms
    Stage 2 - Haversine Screening    :a2, after a1, 0.5ms

    section Dispatch-Svc
    Stage 3 - OSRM Batch Routing I/O  :a3, after a2, 12ms
    Stage 4 - Solver Compute Engine   :a4, after a3, 3ms
    Stage 5 - State & Event Outbox    :a5, after a4, 5ms
```

#### Latency SLA Waterfall Chi tiết (#2):
1.  **Stage 1 - Spatial Filter (`location-svc` Redis H3):** p95 SLA $< 1.5\text{ms}$. Lọc trạng thái `IDLE`, loại bỏ tài xế bận/lock.
2.  **Stage 2 - Haversine Screening (`location-svc`):** p95 SLA $< 0.5\text{ms}$. Chọn top $K=20$ ứng viên Haversine gần nhất.
3.  **Stage 3 - Heavy Scoring & OSRM Routing (`dispatch-svc`):** p95 SLA $< 12\text{ms}$ (bao gồm I/O mạng nội bộ VPC giữa `dispatch-svc` và `map-svc` OSRM Table API).
4.  **Stage 4 - Solver Compute Engine (Greedy / Hungarian / Auction):** p95 SLA $< 3\text{ms}$ (thời gian chạy CPU thuần túy).
5.  **Stage 5 - State Machine & Event Outbox (`dispatch-svc` Redis/Kafka):** p95 SLA $< 5\text{ms}$.
*   **Tổng SLA In-Memory Pipeline (`dispatch-svc`):** p95 $< 22\text{ms}$.
*   **SLA End-to-End (User App $\to$ Gateway $\to$ Dispatch $\to$ Driver App WebSocket):** Target $200\text{ms} \to 800\text{ms}$.

---

### 2.3. Batch Matching, Giao thức Chấp nhận & Quy tắc Router (Batch Acceptance & Router Specs)

#### A. Candidate Generation Pipeline cho Batch Mode (#13):
Trong kịch bản Batch Matching gom đơn ô H3:
*   Mỗi đơn hàng trong số $M$ đơn gom được sẽ chọn top $K=5$ tài xế rảnh gần nhất qua khoảng cách Haversine (tổng số tài xế tập hợp là $N$).
*   `dispatch-svc` phát **1 cuộc gọi duy nhất (Single M-to-N OSRM Table API)** để lấy toàn bộ ma trận ETA $M \times N$ trong 1 request I/O mạng duy nhất, tiết kiệm chi phí I/O.

#### B. Giao thức Chấp nhận Batch (Batch Acceptance Protocol) (#3, F7, F8):
Để giải quyết vấn đề "1 tài xế từ chối làm sụp đổ phương án tối ưu toàn cục", BSM kết hợp 2 cơ chế:
1.  **Nhúng Xác suất Chấp nhận ($P_{accept}$) vào Ma trận Trọng số (Weight Matrix) (#7):** 
    $$\text{Weight}(o_i, d_j) = \text{Score}(o_i, d_j) \cdot P_{accept}(AR, t_{ETA})$$
    với $P_{accept} = \left(\frac{AR}{100}\right) \cdot e^{-0.002 \cdot t_{ETA}}$. Ma trận này được đưa vào Hungarian/Auction Algorithm dưới dạng bài toán **tối đa hóa (maximization)** tổng trọng số toàn cục. Nếu thư viện solver chỉ hỗ trợ minimization theo quy ước cổ điển, bắt buộc áp dụng phép biến đổi $\text{Cost}(o_i,d_j) = C_{max} - \text{Weight}(o_i,d_j)$ trước khi đưa vào solver, với $C_{max} = 130$ (khớp trần $Score_{total} \le 130.0$).
    *Lưu ý thiết kế (#8):* $AR$ được sử dụng có chủ đích ở cả hai vai trò — tín hiệu chất lượng tổng quát trong $Score$ (trọng số $w_2$) và ước lượng xác suất chấp nhận tức thời trong $P_{accept}$ — nhằm chủ động thiên vị batch-matching về phía các tài xế ít có khả năng từ chối. Đây không phải lỗi double-counting; không loại bỏ một trong hai instance khi refactor.
2.  **Bắn đơn Đồng thời (Multi-Offer Sub-batch Dispatch):** Hệ thống gửi lời mời nhận cuốc đồng thời cho các cặp được ghép với TTL = 10 giây. Nếu có tài xế từ chối (`REJECT`), hệ thống giữ nguyên các cặp đã nhận (`ACCEPTED`) và lập tức đưa các đơn bị từ chối vào lượt gom batch nhỏ tiếp theo (Re-solve Sub-batch) sau 500ms.

#### C. Độ phức tạp & Quy tắc Router (#10, #11, F9):
*   **Tính toán Độ phức tạp Hungarian chính xác (#10):** Độ phức tạp chuẩn là $O(\max(M,N)^3)$. Với quy mô trung bình $M=5, N=5$ ($V = \max(M,N) = 5$), số phép tính chỉ là $5^3 = 125$ phép tính (overestimate $8$ lần nếu tính nhầm theo $(M+N)^3 = 1000$).
*   **Quy tắc Router phân tuyến:**
    *   **$V \le 30$:** Chạy **Hungarian Algorithm ($O(V^3)$)**.
    *   **$30 < V \le 200$:** Chạy **Bertsekas' Auction Algorithm ($O(V^2 / \epsilon)$)** song song đa luồng. Tham số bước đấu giá (bidding increment) mặc định là **$\epsilon = 1.0$**. Giá trị $\epsilon = 1.0$ đảm bảo khoảng cách sai số tối ưu không vượt quá $\epsilon \times N \le 30.0$ điểm trên ma trận trọng số, giúp thuật toán hội tụ cực nhanh trong **$1\text{ms} \to 3\text{ms}$** cho $30 < V \le 200$.
    *   **$V > 200$ hoặc Solver Compute $> 10\text{ms}$:** Kích hoạt Timeout Budget Guard, ngắt solver và hạ cấp về **Greedy Single-Assignment ($O(V)$ cho toàn batch, $O(1)$ mỗi cặp gán)** (#F9).

---

### 2.4. Quản lý Chỉ mục Không gian Không khóa & Tối ưu hóa Bộ nhớ Go Engine
*   **Lock-Free Spatial Indexing:** Phân vùng dữ liệu tài xế theo H3 Resolution 8 trên Redis Cluster Shards, xử lý truy vấn đọc không gây khóa bảng ghi (Lock-Free), chịu tải 100.000 QPS.
*   **Zero-Allocation Go Engine:** `dispatch-svc` triển khai Go Worker Pools kết hợp `sync.Pool` tái sử dụng bộ nhớ, ngăn ngừa nghẽn do Garbage Collection (GC pause) khi chạy ở quy mô 100.000 tài xế.

---

### 2.5. Ma trận phối hợp thuật toán & cơ chế điều phối động (Dynamic Dispatch Strategy Decision Matrix)

#### A. Ma trận 6 Bối cảnh Vận hành Thực tế (Cover toàn bộ dải Cung/Cầu) (#4, F10)

| Bối cảnh & Tình huống Vận hành | Tỷ lệ Cung/Cầu (EMA Smoothed Ratio) | Mô hình Chấm điểm Cốt lõi | Thuật toán Gán tài xế & Router Dynamic | Trọng số & Chỉ số Ưu tiên | Target Business & Performance KPIs (#18) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **🟢 Thừa xe / Thấp điểm** | $< 0.8$ (Thừa xe) | Model 2.1 (Non-linear Decay + $P_{accept}$) | **Greedy Single-Assignment ($O(1)$)** - Gán ngay lập tức | $t_{ETA}$, FIFO rảnh lâu ($t_{idle}$) | Solver Latency $< 1\text{ms}$, Đón xe tức thì |
| **🔵 Cân bằng (Balanced Regime)** (#4) | $0.8 \le \text{Ratio} < 1.5$ (Cân bằng) | Model 2.1 Standard Composite Score | **Greedy Single-Assignment + Soft Queue (500ms)** | $t_{ETA}$, $CoR$, $AR$ | Tỷ lệ hủy cuốc $< 5\%$, Latency $< 3\text{ms}$ |
| **🔴 Giờ cao điểm (Downtown)** | $1.5 \le \text{Ratio} \le 3.0$ (Thiếu xe) | Model 2.1 + Dynamic AR/CoR Composite Score | **Localized H3 Matching** (Hungarian nếu $V \le 30$, Auction nếu $30 < V \le 200$) | $CoR$, $AR$, $P_{revenue}$, $S_{aging}$ | Mục tiêu: Giảm 20% tổng ETA vùng (KPI thử nghiệm) |
| **🌧️ Mưa lớn / Ngập lụt / Lễ Tết** | $> 3.0$ (Cực kỳ thiếu xe) | Model 2.1 + Surge Multiplier + Score Decay | **Adaptive Batch Matching** (Cửa sổ co giãn $3\text{s} \to 8\text{s}$) + Auction/Greedy | $C_{vip}$, $P_{revenue}$, Surge *(ghi chú: $L_{cash}$ bổ sung khi Phase 2)* | Tối đa tỷ lệ ghép đơn, hạn chế trôi đơn |
| **🟡 Ngoại thành / Vùng thưa xe** | Mật độ $< 2$ xe/ô H3 | Model 2.1 + Priority Aging Boost | **Greedy + Dynamic H3 Expansion ($k=1 \to 3$)** & MinScore Decay | $t_{wait}$, MinScore Decay, $S_{aging}$ | Bảo đảm SLA cam kết, tuyệt đối không trôi đơn |
| **⚡ Đơn giá trị cao / Khách VIP** | Mọi tỷ lệ | Model 2.1 + Business Revenue Gate | **Filtered Greedy Single-Assignment** (Quality Gate $R_{star} \ge 4.8$) với Fallback Relaxation | $R_{star}$, $CoR$, $P_{revenue}$, $C_{vip}$ | Bảo toàn doanh thu cuốc lớn, giữ chân VIP |

#### Thứ tự Ưu tiên khi Nhiều Điều kiện Đồng thời Đúng (Regime Precedence Order) (F11):
1. ⚡ **VIP / Đơn giá trị cao** — Kiểm tra trước tiên, độc lập với $S_{D,ratio}$; nếu thỏa mãn, áp dụng Business Revenue Gate bất kể chế độ cung/cầu toàn vùng.
2. 🌧️ **Sự kiện khẩn cấp vùng ($S_{D,ratio} > 3.0$)** — Override tiếp theo, do đây là tín hiệu nghẽn trên toàn khu vực.
3. **Chế độ theo $S_{D,ratio}$ (🟢/🔵/🔴)** — Áp dụng nếu không rơi vào (1) hoặc (2).
4. 🟡 **Mật độ H3 cục bộ $<2$ xe/ô** — KHÔNG phải nhánh loại trừ lẫn nhau với (3); là một điều chỉnh cục bộ ($k$-ring expansion) lồng bên trong bất kỳ chế độ nào đã chọn ở bước 1–3, kích hoạt khi mật độ tuyệt đối trong một ô H3 cụ thể quá thấp.

> 🤖 **Lộ trình Học trọng số động (Contextual Bandit / RL - Future Work):** 
> Trọng số ($w_1, w_2, w_3$) có thể được tối ưu tự động theo phân vùng-giờ qua Multi-Objective Reward Function:
> $$R_t = r_{\text{match}} + \lambda_1 \cdot r_{\text{complete}} + \lambda_2 \cdot \left(\frac{\text{Fare}}{\overline{\text{Fare}}}\right) - \lambda_3 \cdot \left(\frac{t_{ETA}}{60}\right) - \lambda_4 \cdot \text{Gini}_{\text{zone}}$$

---

#### B. Cơ chế Bảo vệ & Fallback (Guards & Fallback Protocols)

| Cơ chế Bảo vệ | Điều kiện Kích hoạt | Hành động Fallback & Xử lý | SLA / Giới hạn An toàn |
| :--- | :--- | :--- | :--- |
| **Timeout Budget Guard** (#2, F9) | Thời gian giải Solver (Hungarian/Auction) vượt quá **10ms** | Tự động ngắt thuật toán (Short-circuit solver) và hạ cấp về **Greedy Single-Assignment / Nearest-Neighbor cho các đơn còn lại** ($O(V)$ cho toàn batch, $O(1)$ mỗi lần gán) | Bảo đảm Solver Latency $\le 10\text{ms}$ |
| **Empty Candidate Pool Fallback** | Không có tài xế nào đạt điều kiện lọc cứng (VIP $R_{star} \ge 4.8$ hoặc ô H3 rỗng) | **Bước 1:** Nới lỏng điều kiện lọc (Relaxation Gate: giảm $R_{star}$ xuống $4.5 \to 4.0$). <br> **Bước 2:** Mở rộng vòng lưới H3 ($k$-ring expansion $1 \to 2 \to 3$). | Ngăn ngừa từ chối đơn hàng vô lý |
| **Driver Re-dispatch & Rejection Loop** (#15) | Tài xế bấm từ chối (`REJECT`) hoặc quá hạn 15s (`TIMEOUT`) không phản hồi | **1.** Đưa Driver ID vào `excluded_driver_ids`. <br> **2.** Tách biệt hai bộ đếm vi phạm (#15): <br> - Nếu `REJECT` cố ý: tăng $F_{reject} \implies$ Lock Cooldown 60s (escalate 5m nếu $\ge 3$ lần). <br> - Nếu `TIMEOUT` (sóng yếu 3G/4G): tăng $F_{timeout} \implies$ Lock Cooldown nhẹ 15s để thử lại mạng. <br> **3.** Tăng `order_attempt++`, phát sự kiện retry sau 500ms. | Bảo vệ tài xế vùng sóng yếu không bị phạt oan |
| **Hysteresis State Guard** (F12) | Ratio Cung/Cầu dao động quanh **bất kỳ ranh giới chuyển pha nào giữa 4 vùng chế độ (0.8, 1.5, hoặc 3.0)** | Áp dụng vạch trễ (Hysteresis Band $\pm 0.15$) kết hợp EMA (chu kỳ 30s) tại **cả ba** ranh giới trước khi chuyển kịch bản | Tránh hiện tượng nhấp nháy/dao động kịch bản |

---

#### C. Hạng mục Cần Thử nghiệm Benchmark trước Production (Production Benchmark Requirements)

1. **Ngưỡng kích thước bài toán ($V_{max}$ per H3 Cell):** Quy mô trung bình $V \le 10$. Giới hạn tối đa $V_{max} = 200$. Nếu $V > 30$, Router chuyển Auction Algorithm; nếu $V > 200$, Router hạ cấp Greedy.
2. **Tham số EMA & Hysteresis:** Tuning hệ số làm mịn $\alpha_{EMA} \in [0.1, 0.3]$ và dải trễ $\Delta = 0.15$ trên dữ liệu mô phỏng traffic.
3. **Cửa sổ Batch Thích ứng ($W_{adaptive}$):** Giới hạn $W \in [2\text{s}, 8\text{s}]$ theo QPS thực tế.
4. **Thời gian Timeout phản hồi của Tài xế (Driver Offer TTL):** Thử nghiệm TTL 10s vs 15s trên thiết bị di động thực tế.
5. **Đo đạc p99 Latency của OSRM Batch Routing (#11):** Đo đạc p99 latency thực tế của OSRM Table API trong cùng VPC với `dispatch-svc`, đảm bảo nằm trong ngân sách 12ms.
6. **Kiểm chứng Hằng số Thang điểm ($S_{boost}$ vs $MinScore$) (F13):** Kiểm tra thực tế xem trần $S_{boost} \le 30.0$, ngưỡng khởi điểm $MinScore_{\text{start}} = 60.0$, và **sàn tối thiểu** $MinScore_{\text{floor}} = 30.0$ (theo Mục 3.2) có hoạt động ổn định trên phân phối dữ liệu thật.
7. **Tuning tham số $\epsilon$ của Auction Algorithm:** Thử nghiệm dải giá trị $\epsilon \in [0.1, 2.0]$ trên tập dữ liệu mô phỏng $V \in [30, 200]$ để tìm điểm cân bằng ngọt (Sweet Spot) giữa thời gian tính toán CPU ($< 3\text{ms}$) và độ tiệm cận nghiệm tối ưu toàn cục.

---

## 3. Các tình huống cạnh biên & Quy tắc Thuật toán (Algorithm Edge Cases & Rules)

### 3.1. Trùng điểm số (Tie-Breaking Rules) (F14)
Nếu hai tài xế đạt điểm số bằng nhau:
*   *Ưu tiên 1:* Tài xế có **ETA đường bộ ngắn hơn**.
*   *Ưu tiên 2:* Tài xế có **thời gian chờ đơn (idle time) lâu hơn** (FIFO).
*   *Ưu tiên 3:* Tài xế có **Đánh Giá Sao ($R_{star}$) cao hơn**.

### 3.2. Hạ ngưỡng điểm động & Giới hạn Hạ cấp (MinScore Decay & Terminal State) (#8, F15)
Khi danh sách ứng viên từ `location-svc` không có ai đạt ngưỡng điểm tối thiểu $MinScore_{\text{start}} = 60.0$:
*   Thuật toán tự động giảm ngưỡng điểm tối thiểu qua công thức dạng đóng (closed-form):
    $$MinScore(\text{order\_attempt}) = \max\left(30.0,\ 60.0 \times 0.8^{\text{order\_attempt}}\right)$$
    *Ví dụ cụ thể:* `attempt 0: 60.0` $\to$ `attempt 1: 48.0` $\to$ `attempt 2: 38.4` $\to$ `attempt 3: 30.72` $\to$ `attempt 4: 30.0` (clamped tại sàn) $\to$ `attempt 5: 30.0` (clamped tại sàn).
*   **Trạng thái Kết thúc (Terminal State):** Giới hạn số lần thử tối đa là $\text{MaxOrderAttempt} = 5$. Nếu sau 5 lần thử (`order_attempt = 5`) vẫn không tìm được tài xế đạt điểm sàn $MinScore \ge 30.0$, hệ thống dừng luồng tự động, phát cảnh báo lên hệ thống Vận hành (Ops Alert) và thông báo cho khách hàng: *"Hiện tại các tài xế quanh khu vực đều đang bận, vui lòng thử lại sau ít phút"*.

### 3.3. Xử lý tài xế từ chối hoặc quá hạn phản hồi (Re-dispatch Protocol) (#15, #20)
*   Tài xế bị từ chối sẽ đưa vào `excluded_driver_ids` của đơn hàng đó.
*   Tăng bộ đếm vi phạm tương ứng ($F_{reject}$ hoặc $F_{timeout}$).
*   Tài xế chịu Cooldown Lock tương ứng (60s cho REJECT cố ý, 15s cho TIMEOUT do mạng).
*   Tăng `order_attempt++`, phát sự kiện `booking.retry_requested` đưa đơn về luồng điều phối sau 500ms.
