# 📜 BSM ALGORITHM SCORING ENGINE - INTERFACE SPECIFICATION
> **Phiên bản:** v1.0 | **Đơn vị phát triển:** Algorithm Team | **Scope:** Pure Decision Engine Contract

---

## 🎯 1. BẢN CHẤT GIAO TIẾP (ENGINE NATURE)

* **BSM Scoring Engine** là một **Pure Math Function (Stateless & Zero Network I/O)**.
* **Thời gian xử lý (SLA Latency):** **$\le 0.003\text{ ms}$** cho tập 30 tài xế.
* **Nguyên tắc:** Engine KHÔNG thực hiện bất kỳ kết nối Database, Redis hay HTTP/gRPC nào. Mọi dữ liệu phải được `dispatch-svc` lắp ráp sẵn trên RAM và nạp trực tiếp qua Struct Go.

---

## 📥 2. YÊU CẦU ĐẦU VÀO (INPUT CONTRACT FOR DISPATCH-SVC)

`dispatch-svc` bắt buộc phải gom đủ và nạp đúng 2 Struct sau vào hàm `FindAndRankDriversAdvanced(candidates, ctx, now)`:

### 2.1. Struct `BookingContext` (Dữ liệu Ngữ cảnh Đơn hàng)
1. `EstimatedFare` (Float64): Giá cước ước tính từ `order-svc`.
2. `TripDistanceMeters` (Float64): Quãng đường chuyến đi (từ điểm đón A đến điểm trả B).
3. `IsVIP` (Boolean): Cờ xác định Khách Hàng VIP.
4. `Attempt` (Int): Số lần đã thử ghép cuốc (`0, 1, 2...`).
5. `CreatedAt` (Time): Thời điểm khách bấm đặt đơn.
6. `CurrentRadiusMeters` (Float64): Bán kính đang quét hiện tại.
7. `InitialRadiusMeters` & `RadiusExpansionRate` & `MaxRadiusMeters`: Tham số bán kính.
8. `ExcludedDriverIDs` ([]String): Danh sách ID tài xế bị loại trừ từ các lượt thử trước.
9. `ServiceType` (String): Loại dịch vụ xe (`BIKE`, `CAR_4SEAT`, `CAR_7SEAT`).
10. `PaymentMethod` (String): Hình thức thanh toán (`CASH`, `WALLET`, `CARD`).

### 2.2. Danh sách `[]Candidate` (Dữ liệu Ứng viên Tài xế & Đường đi)
Mỗi ứng viên trong mảng `candidates` phải có đủ thông tin:
* **Tài xế (`candidate.Driver`):** `ID`, `Rating`, `AcceptanceRate`, `CompletionRate`, `CancellationRate`, `IdleTimeSeconds`, `WalletBalance`, `VehicleType`, `IsIdle`, `CooldownUntil`.
* **Đường đi (`candidate.Route`):** `ETASeconds`, `BarrierCount`, `RoadDistanceMeters`.

---

## 📤 3. KHẾ ƯỚC ĐẦU RA (OUTPUT CONTRACT RETURNED BY ENGINE)

Engine tính toán và trả về duy nhất 1 Struct **`DispatchDecision`** chứa đầy đủ kết quả:

```go
type DispatchDecision struct {
    TopCandidate        *ScoringResult  // Tài xế Xếp hạng #1 đạt điểm sàn (MinScore)
    AllResults          []ScoringResult // Danh sách TOÀN BỘ tài xế đã chấm điểm & xếp hạng giảm dần
    ShouldExpandRadius  bool            // Trả về true nếu KHÔNG CÓ tài xế nào đạt điểm sàn MinScore
    SuggestedNextRadius float64         // Bán kính gợi ý cho lượt quét tiếp theo (khi ShouldExpandRadius = true)
    EffectiveMinScore   float64         // Ngưỡng điểm sàn áp dụng cho attempt này
}
```

### 💡 Hướng dẫn cho `dispatch-svc` khai thác kết quả:
* **Trường `TopCandidate`:** Dùng cho lựa chọn tài xế tối ưu nhất (#1).
* **Trường `AllResults`:** Danh sách đã được sắp xếp từ điểm cao xuống điểm thấp (Rank 1, Rank 2, Rank 3...). `dispatch-svc` tự khai thác danh sách này theo chiến lược gán cuốc của team.
* **Trường `ShouldExpandRadius` & `SuggestedNextRadius`:** Khi `ShouldExpandRadius == true`, `dispatch-svc` tự dùng `SuggestedNextRadius` để tăng `Attempt++` và gọi lại `location-svc` quét bán kính mới.

---

## 🎛️ 4. CHÍNH SÁCH QUẢN LÝ CẤU HÌNH THUẬT TOÁN (CONFIG INTERFACE)

* **Lưu trữ:** Package `pkg/scoring/config.go` (`ConfigManager`) nằm trên RAM của `dispatch-svc`.
* **Tham số nạp:** Quản lý trọng số ($W_1, W_2, W_3$), hệ số suy giảm ETA ($\alpha$), rào cản ($w_{barrier}$), ngưỡng điểm sàn ($BaseMinScore, MinScoreFloor$), và mốc cước trung bình (`FareAvgZoneHour`).
* **Hot-Reload:** Engine cung cấp hàm `GetConfigManager().UpdateConfig(serviceType, newCfg)` để `dispatch-svc` cập nhật trọng số tức thì qua RAM mà không cần restart server.

---

### 🎯 TÓM LẠI:
> *"Phía Algorithm cam kết giữ latency $\le 0.003\text{ ms}$ và trả về kết quả xếp hạng tối ưu `DispatchDecision`. Việc thu thập dữ liệu đầu vào và xử lý luồng gán cuốc phía sau thuộc trách nhiệm của `dispatch-svc` và `location-svc`."*
