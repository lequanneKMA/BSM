# BSM Dispatch Algorithm — Review v2 (đã điều chỉnh sau phản biện)

> Bản v1 có 20 issue. Sau phản biện, severity của một số issue được điều chỉnh, một số bị thu hẹp phạm vi, một số bị rút gần như toàn bộ. Bản này là kết quả cuối.

## Tóm tắt thay đổi so với v1

| # | Chủ đề | v1 | v2 | Lý do đổi |
|---|---|---|---|---|
| 3 | Batch acceptance protocol | Critical | High | Đây là điểm giao giữa thuật toán và orchestration, không thuần thuật toán |
| 5 | Metric chưa dùng | High ("decorative") | High ("Phase 2 metrics") | Từ "decorative" gây hiểu lầm là vô dụng |
| 6 | VIP decay theo attempt | High (bug) | Medium (thiếu rationale) | Là trade-off hợp lệ giữa hai philosophy, không phải bug |
| 12 | Barrier double-count | Medium (kết luận chắc) | Medium (cần định nghĩa rõ) | Kết luận double-count quá mạnh tay nếu $B_{barrier}$ đại diện chi phí UX độc lập |
| 14 | AR pressure | Medium | Low-Medium (thu hẹp) | Việc có dùng AR là product policy; issue thật chỉ là thiếu exemption cho decline hợp lệ |
| 15 | Timeout = Reject | Medium | Low (risk-flag) | Mức độ phạt là business decision, không phải defect thuật toán |
| 16 | Rating² | Medium | Rút thành câu hỏi xác nhận | Non-linear transform theo feature là bình thường trong ranking, không cần chứng minh toán học |
| 10 | Hungarian complexity (M+N)³ | Medium | Medium (giữ nguyên) | Xác nhận đúng, không đổi |

---

## 🔴 Critical (3)

### #1 — Weight $w_1, w_2, w_3, w_{barrier}$ chưa định nghĩa
- **Vị trí:** §2.1
- **Vấn đề:** Không có default value, không có invariant. Claim "$Score_{core} \in [0,100]$" chỉ đúng nếu $w_1+w_2+w_3 \le 1$, nhưng điều này chưa từng được nêu.
- **Fix:** Công bố default weight cụ thể + enforce invariant $w_1+w_2+w_3=1$ tại config-load time.

### #2 — SLA mâu thuẫn nội bộ
- **Vị trí:** §2.5 sidebar (< 5ms) vs. Guards table §B (15ms timeout, ≤20ms guarantee)
- **Vấn đề:** Ba con số (5 / 15 / 20 ms) cho cùng một pipeline stage, chưa rõ cái nào là solver, compute, hay end-to-end.
- **Fix:** Viết lại thành latency waterfall theo từng stage (candidate fetch → profile → OSRM → solve), có p50/p95/p99 riêng.

### #4 — Khoảng tỷ lệ cung/cầu 0.8 → 1.5 không có context nào cover
- **Vị trí:** §2.5 Table A
- **Vấn đề:** Bảng có `<0.8`, `1.5→3.0`, `>3.0` — một đơn hàng ở tỷ lệ 1.2 (trạng thái phổ biến nhất của marketplace cân bằng) rơi vào khoảng trống, không có model/algorithm nào được gán.
- **Fix:** Thêm row "balanced" cho khoảng 0.8–1.5 với model/algorithm riêng.

---

## 🟠 High (4)

### #3 — Batch matching (Hungarian/Auction) chưa có acceptance protocol
- **Vị trí:** §2.3 vs. §3.3
- **Vấn đề:** Hungarian/Auction giải bài toán giả định tất cả các cặp trong batch đều commit. §3.3 lại mô tả dispatch kiểu tuần tự (1 offer, 15s, reject → retry). Khi một driver trong batch reject, "tối ưu tuyệt đối toàn cục" mà §2.3 quảng cáo không còn đúng nghĩa.
- **Lưu ý:** Cách xử lý cụ thể (multi-offer, re-solve) có thể thuộc về orchestration layer — nhưng nếu chọn hướng model hóa $P_{accept}$ vào cost function, đó là quyết định thuật toán thật sự, cần thiết kế rõ.
- **Fix:** Chọn 1 trong 2: (a) đưa $P_{accept}(AR, t_{ETA})$ vào cost matrix, hoặc (b) multi-offer đồng thời + re-solve sub-batch khi có reject.

### #5 — ~10/19 metric trong §1 chưa được dùng ở §2
- **Vị trí:** §1 vs. §2.1/§2.5
- **Vấn đề:** $L_{tier}$, $N_{trips}$, $F_{penalty}$ (scoring), $d_{road}$, $\rho_{traffic}$, $M_{commission}$, $C_{fuel}$, $L_{cash}$, $V_{velocity}$, $R_{retention}$ đều có mô tả business rationale chi tiết ở §1 nhưng không xuất hiện trong công thức hay decision matrix.
- **Lưu ý:** Đây là "Phase 2 metrics / future candidate features" — không phải vô dụng, chỉ chưa wire vào.
- **Fix:** Đánh dấu rõ "Phase 2 — chưa active" trong doc, để implementer không giả định chúng đang chạy.

### #7 — "FareRatio" dùng trong $P_{revenue}$ nhưng chưa định nghĩa
- **Vị trí:** §2.1, $P_{revenue} = \min(5.0, \text{FareRatio} \times CoR)$
- **Vấn đề:** Không rõ là `fare/avgFare`, `fare/maxFare`, hay công thức nào khác.
- **Fix:** Định nghĩa rõ FareRatio, kèm ví dụ số như đã làm với $\alpha_{ETA}$.

### #8 — MinScore Decay không có floor/max-attempt
- **Vị trí:** §3.2
- **Vấn đề:** $60 \to 48 \to 38.4 \to \dots$ không có giới hạn dưới. Sau ~15-20 lần retry, ngưỡng gần như bằng 0.
- **Fix:** Thêm `MinScore >= 30` hoặc `MaxRetry`, kèm terminal state (escalate ops / báo khách "không tìm được tài xế").

---

## 🟡 Medium (7)

### #6 — $S_{VIP}$ giảm theo attempt: thiếu rationale, không phải bug
- **Vị trí:** §2.1, $S_{VIP} = 10.0 \times 0.5^{attempt}$
- **Vấn đề:** Đây là trade-off hợp lệ (giảm boost để tránh ép match giả tạo khi VIP liên tục bị reject) — không sai tuyệt đối. Nhưng doc không nói rõ đây là chủ đích "bảo vệ VIP" (nên tăng) hay "tránh starvation do ép boost" (nên giảm). Hai cách đọc dẫn tới implementation ngược nhau.
- **Fix:** Nêu rõ philosophy nào được chọn và lý do.

### #9 — $S_{aging}$ bão hòa ở 50 giây
- **Vị trí:** §2.1
- **Vấn đề:** Đơn chờ 1 phút và đơn chờ 10 phút nhận cùng +10 boost. Không còn phân biệt sau ~50s.
- **Fix:** Rescale hệ số để phù hợp với phân phối thời gian chờ thực tế (đặc biệt ở "Ngoại thành").

### #10 — Complexity Hungarian dùng $(M+N)^3$ thay vì $\max(M,N)^3$
- **Vị trí:** §2.3.A
- **Vấn đề:** Chuẩn Hungarian là $O(\max(M,N)^3)$. Dùng $V=M+N$ rồi lập phương overestimate ~8x với M≈N.
- **Fix:** Tính lại theo $\max(M,N)$.

### #11 — Benchmark "<1μs" nêu như fact, chưa đo thật
- **Vị trí:** §2.3.A
- **Vấn đề:** Là FLOP-count lý thuyết, chưa tính overhead runtime Go, allocation, và đặc biệt chưa tính chi phí build cost-matrix (OSRM calls) — thứ nhiều khả năng dominate latency thật.
- **Fix:** Thay bằng số liệu profiling thật (p50/p95/p99), tách riêng thời gian solve và thời gian build matrix.

### #12 — $G_{barrier}$ và OSRM ETA có thể double-count — cần định nghĩa rõ, chưa kết luận chắc
- **Vị trí:** §2.1 vs. §1.1
- **Vấn đề:** Theo định nghĩa gốc ở §1.1 (điểm quay đầu, dải phân cách, cầu vượt, đường tàu — đều là đặc điểm route thực tế), khả năng cao đã được phản ánh trong $t_{ETA}$ (OSRM), nên $G_{barrier}$ phạt thêm lần 2. Nhưng nếu $B_{barrier}$ đại diện chi phí UX độc lập (không tốn thời gian nhưng gây khó chịu), thì tách ra là hợp lý.
- **Fix:** Định nghĩa rõ $B_{barrier}$ đo cái gì *độc lập* với route-distance.

### #13 — Batch cost-matrix chưa có pipeline build
- **Vị trí:** §2.2 (cap 20 candidate, cho single-order) vs. §2.3 (batch M×N, V tới 200)
- **Vấn đề:** Batch Hungarian cần cost matrix đầy đủ, đòi hỏi ETA thật cho hàng nghìn cặp order-driver. Pipeline 20-candidate ở §2.2 không đủ cho việc này, và chi phí OSRM cho batch chưa được budget ở đâu.
- **Fix:** Định nghĩa rõ candidate-generation path cho batch mode + thêm chi phí OSRM vào latency waterfall (#2).

### #14 — AR dùng trực tiếp trong scoring, thiếu exemption cho decline hợp lệ
- **Vị trí:** §1.2, §2.1
- **Vấn đề:** Việc *có dùng* AR là product policy, không phải algorithm bug. Nhưng một khi đã dùng, công thức hiện tại không phân biệt decline hợp lệ (an toàn, hết ca) với decline tùy tiện — đây là gap ở tầng implementation.
- **Fix:** Loại trừ decline có gắn cờ hợp lệ khỏi công thức AR, hoặc cap ảnh hưởng của AR.

---

## 🟢 Low (5)

### #15 — REJECT = TIMEOUT trong $F_{penalty}$/cooldown — risk-flag, không phải defect
- **Vị trí:** Guards table §B, §3.3, §C.4
- **Vấn đề:** Mức phạt là business decision. Nhưng chính §C.4 đã thừa nhận TIMEOUT có thể do trễ mạng 3G/4G — nếu không tách riêng, tài xế vùng sóng yếu chịu thiệt.
- **Fix:** Cân nhắc tách counter, để lại cho product quyết định mức độ.

### #17 — Unit của $t_{ETA}$ ghi mơ hồ ("giây/phút")
- **Vị trí:** §1.1 vs. §2.1
- **Fix:** Ghi rõ "seconds" trong §1.1, bỏ "giây/phút".

### #18 — Vài số benchmark ("giảm 20% ETA", "cắt 99.95% chi phí") chưa có nguồn
- **Vị trí:** §2.2, §2.5 table
- **Fix:** Đưa vào danh sách "cần benchmark" ở §C như 6 tham số kia, thay vì nêu như fact.

### #19 — $P_{revenue}$: narrative (§1.5) nhiều hơn formula (§2.1)
- **Vị trí:** §1.5 vs. §2.1
- **Fix:** Đồng bộ formula và định nghĩa (formula thiếu phần "lịch sử hủy chuyến" mà §1.5 nhắc tới).

### #20 — Biến "attempt" dùng chung cho nhiều mục đích, chưa rõ có phải cùng 1 counter
- **Vị trí:** §2.1 ($S_{VIP}$), §3.2 (MinScore), §3.3/Guards (re-dispatch)
- **Fix:** Đặt tên/scope rõ ràng cho từng counter. Nếu dùng chung, cần giải quyết #6 trước (tránh hiệu ứng chồng chéo: đơn VIP bị reject vừa được hạ ngưỡng chất lượng vừa bị giảm boost VIP cùng lúc).

---

## ❓ Câu hỏi xác nhận (không còn là issue)

### $(R_{star}/5.0)^2$ trong khi $AR$, $CoR$ để tuyến tính
Non-linear transform theo từng feature (log/square/sigmoid) là chuyện bình thường trong ranking system, không cần chứng minh toán học — chỉ cần benchmark/A-B test hỗ trợ. Câu hỏi còn lại chỉ là: đây là quyết định có chủ đích (có data support) hay là lựa chọn ngẫu nhiên chưa qua kiểm chứng?

---

## Top 10 cải tiến (đã cập nhật thứ tự theo severity mới)

1. Công bố weight cụ thể ($w_1, w_2, w_3, w_{barrier}$) + enforce invariant $w_1+w_2+w_3=1$.
2. Gộp 3 con số SLA (5/15/20ms) thành 1 latency waterfall rõ ràng theo stage.
3. Định nghĩa "balanced regime" cho khoảng tỷ lệ cung/cầu 0.8–1.5.
4. Thiết kế acceptance protocol cho batch matching (embed $P_{accept}$ vào cost matrix, hoặc multi-offer + re-solve).
5. Đánh dấu rõ các metric chưa active là "Phase 2", tránh implementer giả định sai.
6. Định nghĩa rõ FareRatio, kèm ví dụ số.
7. Thêm floor + max-attempt cho MinScore Decay, có terminal state.
8. Nêu rõ philosophy cho $S_{VIP}$ decay (bảo vệ VIP hay tránh ép boost giả tạo).
9. Fix complexity model Hungarian ($\max(M,N)$ thay vì $M+N$) + thay benchmark lý thuyết bằng số liệu profiling thật.
10. Rescale $S_{aging}$ để phù hợp phân phối thời gian chờ thực tế.

---

## Đánh giá cuối

**Readiness: 3/10** — giữ nguyên so với v1. Ba issue Critical (#1, #2, #4) đủ để chặn implementation, không phụ thuộc vào các điều chỉnh severity ở nhóm còn lại.

**Approve cho production? Không.** Cần giải quyết tối thiểu 3 Critical + 4 High trước khi implement, sau đó chạy benchmark thật theo §C trước khi đưa vào production traffic.
