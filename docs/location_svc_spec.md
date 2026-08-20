# Đặc Tả Kỹ Thuật: location-svc (Spatial Engine)

Dành cho nhóm phát triển dịch vụ định vị và không gian (**`location-svc`**).

---

## 📍 1. Tổng Quan Nhiệm Vụ (Scope of Responsibility)

`location-svc` là dịch vụ chuyên biệt xử lý các tác vụ không gian (Spatial Computing) với hiệu năng cao. Dịch vụ này không thực hiện việc chấm điểm hay nghiệp vụ ghép đơn, chỉ tập trung vào:
1.  Nhận và lưu trữ tọa độ GPS thời gian thực của tài xế.
2.  Truy vấn tìm kiếm các tài xế lân cận theo mô hình phân vùng lưới lục giác H3.
3.  Tính toán ma trận khoảng cách đường bộ thực tế (Distance & ETA) bằng cách gọi tích hợp với `map-svc` (lõi OSRM).

---

## 💾 2. Cơ Sở Dữ Liệu & In-Memory Spatial Cache

Để đạt độ trễ đọc/ghi cực thấp (<2ms), toàn bộ vị trí động của tài xế được lưu trữ trên bộ nhớ in-memory của **Redis**:

*   **Redis Key cập nhật vị trí:** `driver:location:{driver_id}`
    *   *Kiểu dữ liệu:* Hash hoặc GeoSet.
    *   *Payload lưu trữ:*
        ```json
        {
          "lat": 21.0285,
          "lng": 105.8542,
          "updated_at": 1790487000
        }
        ```
*   **H3 Spatial Grid Index:**
    *   Hệ thống áp dụng thư viện **Uber H3 ở Resolution 8** để gom nhóm tài xế.
    *   Mỗi khi GPS của tài xế cập nhật, dịch vụ chuyển tọa độ `(lat, lng)` sang mã lục giác H3 Index (ví dụ: `88411c2b57fffff`) và đưa tài xế vào Set tương ứng của H3 Cell đó trên Redis.

---

## ⚙️ 3. Cơ Chế Co Giãn Bán Kính Theo Vòng Lục Giác (Dynamic H3 Ring Expansion)

Khi nhận yêu cầu tìm tài xế lân cận với tham số `attempt` (lượt thử gán), `location-svc` tự động mở rộng phạm vi tìm kiếm theo thuật toán k-ring lục giác bao quanh cell của khách hàng:

```
          / \ / \
         | 2 | 2 |
        / \ / \ / \
       | 2 | 1 | 2 |
      / \ / \ / \ / \
     | 2 | 1 | 0 | 1 | 2 |  <-- (0: Cell gốc của khách, 1: k-ring 1, 2: k-ring 2)
      \ / \ / \ / \ /
       | 2 | 1 | 2 |
        \ / \ / \ /
         | 2 | 2 |
          \ / \ /
```

*   **attempt = 0:** Quét `k-ring = 1` (cell gốc của khách + 6 cell liền kề). Bán kính tương đương **~1.5 km**.
*   **attempt = 1:** Quét `k-ring = 2` (cell gốc + 18 cell bao quanh). Bán kính tương đương **~2.25 km**.
*   **attempt = 2:** Quét `k-ring = 3` (cell gốc + 36 cell bao quanh). Bán kính tương đương **~3.37 km**.
*   **attempt = 3 trở đi (Lượt thử tối đa):** Quét `k-ring = 4` (cell gốc + 60 cell bao quanh). Bán kính tương đương **~5.0 km** (giới hạn quét trần của hệ thống).

---

## 🚗 4. Tích Hợp `map-svc` Tính Ma Trận ETA Đường Bộ (Batch Routing)

Sau khi tìm được tập hợp tài xế đang rảnh trong vùng lưới lục giác mở rộng, `location-svc` thực hiện:
1.  Gom tối đa 50 tài xế gần nhất về mặt khoảng cách chim bay.
2.  Gửi yêu cầu tính toán ma trận hành trình tới `map-svc` (OSRM Table API) thông qua API:
    `POST /api/v1/routes/batch`
    *   *Payload:* `{ "pickup_coords": [lat, lng], "driver_coords": [[lat1, lng1], [lat2, lng2], ...] }`
3.  `map-svc` phản hồi ma trận ETA đường bộ (giây) và khoảng cách thực tế (mét).
4.  `location-svc` sắp xếp danh sách tài xế theo ETA đường bộ tăng dần, giữ lại tối đa 20 tài xế tốt nhất có ETA hợp lệ để trả về cho `dispatch-svc`.

---

## 🔌 5. API Contracts (Thiết Kế Cổng Giao Tiếp)

### 5.1 API Cập Nhật Vị Trí của Tài Xế
*   **Endpoint:** `POST /api/v1/locations/update`
*   **Tần suất gọi:** Định kỳ 3-5 giây từ Driver App.
*   **Request Payload:**
    ```json
    {
      "driver_id": "drv_10293",
      "lat": 21.028511,
      "lng": 105.854223,
      "status": "IDLE"
    }
    ```
*   **Response:** `200 OK`

### 5.2 API Lấy Danh Sách Tài Xế Lân Cận (Dành cho `dispatch-svc` gọi)
*   **Endpoint:** `GET /api/v1/locations/nearby`
*   **Query Parameters:**
    *   `lat` (double): Vĩ độ đón khách.
    *   `lng` (double): Kinh độ đón khách.
    *   `attempt` (int): Lượt thử điều phối hiện tại (0 đến 5).
    *   `vehicle_type` (string): Loại xe yêu cầu (`motorbike`, `car_4`, `car_7`).
*   **Response Payload:**
    ```json
    {
      "search_radius_meters": 2250,
      "drivers": [
        {
          "driver_id": "drv_10293",
          "lat": 21.029100,
          "lng": 105.855100,
          "eta": 180.5,
          "distance_meters": 1200.0
        },
        {
          "driver_id": "drv_99210",
          "lat": 21.031200,
          "lng": 105.859300,
          "eta": 290.0,
          "distance_meters": 1850.0
        }
      ]
    }
    ```
