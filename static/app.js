// Global State
let map;
let ws;
let drivers = new Map();
let bookings = new Map();
let driverMarkers = new Map();
let bookingMarkers = new Map();
let currentPromptToken = null;
let currentPromptBookingId = null;
let currentPromptDriverId = null;
let promptCountdownInterval = null;

let tempPickupMarker = null;
let tempPickupCoords = null;

// Initialize App
document.addEventListener("DOMContentLoaded", () => {
  initMap();
  initWebSocket();
  initEventListeners();
});

// Initialize Leaflet Map Centered at Hanoi
function initMap() {
  const hanoiPos = [21.0285, 105.8542];
  map = L.map("map").setView(hanoiPos, 13);

  // Carto Dark Matter High-Tech Map Tiles
  L.tileLayer("https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png", {
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>',
    subdomains: "abcd",
    maxZoom: 19,
  }).addTo(map);

  // Map Click Handler -> Select Pickup Location
  map.on("click", (e) => {
    setTempPickupLocation(e.latlng.lat, e.latlng.lng);
  });
}

function setTempPickupLocation(lat, lng) {
  tempPickupCoords = { lat: lat, lng: lng };

  const pinIcon = L.divIcon({
    html: `<div style="color: #00f0ff; font-size: 34px; filter: drop-shadow(0 2px 10px rgba(0, 240, 255, 0.8));"><i class="fa-solid fa-location-dot fa-bounce"></i></div>`,
    className: "custom-leaflet-icon",
    iconSize: [34, 34],
    iconAnchor: [17, 34],
  });

  if (tempPickupMarker) {
    tempPickupMarker.setLatLng([lat, lng]);
  } else {
    tempPickupMarker = L.marker([lat, lng], { icon: pinIcon, draggable: true }).addTo(map);
    tempPickupMarker.on("dragend", (e) => {
      const pos = e.target.getLatLng();
      tempPickupCoords = { lat: pos.lat, lng: pos.lng };
      document.getElementById("pickup-coords-text").innerText = `Tọa độ: (${pos.lat.toFixed(4)}, ${pos.lng.toFixed(4)})`;
    });
  }

  document.getElementById("pickup-coords-text").innerText = `Tọa độ: (${lat.toFixed(4)}, ${lng.toFixed(4)})`;
  document.getElementById("pickup-confirm-card").classList.remove("hidden");
}

function clearTempPickup() {
  if (tempPickupMarker) {
    map.removeLayer(tempPickupMarker);
    tempPickupMarker = null;
  }
  tempPickupCoords = null;
  document.getElementById("pickup-confirm-card").classList.add("hidden");
}

function initWebSocket() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsUrl = `${protocol}//${window.location.host}/ws`;

  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    addLogLine("[System] Kết nối WebSocket thành công với Server Hà Nội!", "success");
  };

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleWSMessage(msg);
    } catch (err) {
      console.error("Failed to parse WS message:", err);
    }
  };

  ws.onclose = () => {
    addLogLine("[System] Mất kết nối WebSocket. Đang kết nối lại sau 2s...", "warn");
    setTimeout(initWebSocket, 2000);
  };
}

function handleWSMessage(msg) {
  switch (msg.type) {
    case "INIT":
      if (msg.payload.drivers) {
        msg.payload.drivers.forEach((d) => updateDriverState(d));
      }
      if (msg.payload.bookings) {
        msg.payload.bookings.forEach((b) => updateBookingState(b));
      }
      if (msg.payload.stats) {
        updateStats(msg.payload.stats);
      }
      break;

    case "DRIVER_UPDATED":
      updateDriverState(msg.payload);
      break;

    case "DRIVER_REMOVED":
      removeDriverState(msg.payload);
      break;

    case "BOOKING_UPDATED":
      updateBookingState(msg.payload);
      break;

    case "BOOKING_REMOVED":
      removeBookingState(msg.payload);
      break;

    case "LOG":
      addLogLine(`[${msg.payload.time}] ${msg.payload.message}`, msg.payload.level);
      break;

    case "STATS":
      updateStats(msg.payload);
      break;
  }
}

let renderDriversTimer = null;
function requestRenderDriversList() {
  if (renderDriversTimer) return;
  renderDriversTimer = setTimeout(() => {
    renderDriversTimer = null;
    renderDriversListInternal();
  }, 300);
}

let renderBookingsTimer = null;
function requestRenderBookingsList() {
  if (renderBookingsTimer) return;
  renderBookingsTimer = setTimeout(() => {
    renderBookingsTimer = null;
    renderBookingsListInternal();
  }, 300);
}

function renderDriversList() {
  requestRenderDriversList();
}

function renderBookingsList() {
  requestRenderBookingsList();
}

function updateDriverState(driver) {
  drivers.set(driver.id, driver);

  let iconColor = "#10b981"; // IDLE -> Green
  if (driver.status === "ASSIGNING") iconColor = "#f59e0b"; // ASSIGNING -> Yellow
  if (driver.status === "BUSY") iconColor = "#a855f7"; // BUSY -> Purple

  if (driverMarkers.has(driver.id)) {
    const marker = driverMarkers.get(driver.id);
    marker.setLatLng([driver.position.lat, driver.position.lng]);
    marker.setIcon(createDriverIcon(iconColor, driver.status, driver.vehicleType));
  } else {
    const marker = L.marker([driver.position.lat, driver.position.lng], {
      icon: createDriverIcon(iconColor, driver.status, driver.vehicleType),
      draggable: true,
    }).addTo(map);

    const walletTxt = (driver.walletBalance || 0).toLocaleString("vi-VN") + "đ";
    marker.bindPopup(`
      <b>${driver.name}</b> (${driver.vehicleType || "Xe Máy"})<br>
      Trạng thái: <b>${driver.status}</b><br>
      💰 Ví: <b>${walletTxt}</b> | ⏱️ Lái: <b>${driver.drivingMinutes || 0}m</b><br>
      ⭐ Rating: ${driver.rating || 4.8} | 🎯 ${driver.acceptanceRate || 95}%
    `);

    marker.on("dragend", (e) => {
      const newPos = e.target.getLatLng();
      driver.position = { lat: newPos.lat, lng: newPos.lng };
      fetch(`/api/drivers/${driver.id}/position`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ position: driver.position }),
      });
    });

    driverMarkers.set(driver.id, marker);
  }

  requestRenderDriversList();
}

function removeDriverState(driverId) {
  drivers.delete(driverId);
  if (driverMarkers.has(driverId)) {
    map.removeLayer(driverMarkers.get(driverId));
    driverMarkers.delete(driverId);
  }
  requestRenderDriversList();
}

function createDriverIcon(color, status, vehicleType) {
  const pulseClass = status === "ASSIGNING" ? "pulse-icon" : "";
  let iconClass = "fa-car";
  if (vehicleType && (vehicleType.includes("Xe Máy") || vehicleType.includes("Bike"))) {
    iconClass = "fa-motorcycle";
  } else if (vehicleType && (vehicleType.includes("7 chỗ") || vehicleType.includes("Luxury"))) {
    iconClass = "fa-van-shuttle";
  }

  const html = `
    <div style="background-color: ${color}; width: 30px; height: 30px; border-radius: 50%; border: 2px solid #ffffff; display: flex; align-items: center; justify-content: center; box-shadow: 0 0 12px ${color};" class="${pulseClass}">
      <i class="fa-solid ${iconClass}" style="color: #0b0f19; font-size: 13px;"></i>
    </div>
  `;
  return L.divIcon({
    html: html,
    className: "custom-leaflet-icon",
    iconSize: [30, 30],
    iconAnchor: [15, 15],
  });
}

function updateBookingState(booking) {
  bookings.set(booking.id, booking);

  if (booking.status === "CANCELLED" || booking.status === "FAILED" || booking.status === "COMPLETED") {
    if (booking.driverId) {
      stopDriverMovementAnimation(booking.driverId);
    }
    drivers.forEach((d) => {
      if (d.currentBookingId === booking.id) {
        stopDriverMovementAnimation(d.id);
      }
    });
  }

  let iconColor = "#a855f7"; // PENDING -> Purple
  if (booking.status === "ASSIGNING") iconColor = "#f59e0b"; // ASSIGNING -> Yellow
  if (booking.status === "ACCEPTED") iconColor = "#00f0ff"; // ACCEPTED -> Cyan
  if (booking.status === "COMPLETED") iconColor = "#10b981"; // COMPLETED -> Green
  if (booking.status === "CANCELLED" || booking.status === "FAILED") iconColor = "#f43f5e"; // FAILED -> Red

  if (bookingMarkers.has(booking.id)) {
    const layerGroup = bookingMarkers.get(booking.id);
    layerGroup.clearLayers();
    addBookingToLayerGroup(booking, layerGroup, iconColor);
  } else {
    const layerGroup = L.layerGroup().addTo(map);
    addBookingToLayerGroup(booking, layerGroup, iconColor);
    bookingMarkers.set(booking.id, layerGroup);
  }

  requestRenderBookingsList();
}

// Cache OSRM Route Data to prevent duplicate API hits
const osrmRouteCache = new Map();

async function fetchRealRoadPath(fromPos, toPos) {
  const cacheKey = `${fromPos.lat.toFixed(4)},${fromPos.lng.toFixed(4)}_${toPos.lat.toFixed(4)},${toPos.lng.toFixed(4)}`;
  if (osrmRouteCache.has(cacheKey)) {
    return osrmRouteCache.get(cacheKey);
  }

  try {
    const url = `https://router.project-osrm.org/route/v1/driving/${fromPos.lng},${fromPos.lat};${toPos.lng},${toPos.lat}?overview=full&geometries=geojson`;
    const response = await fetch(url);
    const data = await response.json();
    if (data.code === "Ok" && data.routes && data.routes.length > 0) {
      const latLngs = data.routes[0].geometry.coordinates.map((c) => [c[1], c[0]]);
      osrmRouteCache.set(cacheKey, latLngs);
      return latLngs;
    }
  } catch (e) {
    console.warn("OSRM routing API unavailable, fallback to direct line:", e);
  }

  const fallback = [[fromPos.lat, fromPos.lng], [toPos.lat, toPos.lng]];
  return fallback;
}

// Active Driver Real-Road Movement Animations
const activeMovements = new Map();

function startDriverMovementAnimation(driverId, pathCoords, bookingId) {
  const movementKey = `${driverId}_${bookingId}`;
  if (activeMovements.has(movementKey)) {
    return;
  }

  stopDriverMovementAnimation(driverId);
  if (!pathCoords || pathCoords.length < 2) return;

  let stepIndex = 0;
  const totalSteps = pathCoords.length;

  const intervalId = setInterval(() => {
    if (stepIndex >= totalSteps) {
      clearInterval(intervalId);
      activeMovements.delete(movementKey);
      if (drivers.has(driverId)) {
        const d = drivers.get(driverId);
        addLogLine(`🏁 [Đã Đón Khách] Tài xế ${d.name} đã di chuyển tới điểm đón!`, "success");
      }
      return;
    }

    const currentCoords = pathCoords[stepIndex];
    const newPos = { lat: currentCoords[0], lng: currentCoords[1] };

    if (drivers.has(driverId)) {
      const driver = drivers.get(driverId);
      driver.position = newPos;

      if (driverMarkers.has(driverId)) {
        driverMarkers.get(driverId).setLatLng([newPos.lat, newPos.lng]);
      }

      fetch(`/api/drivers/${driverId}/position`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ position: newPos }),
      }).catch(() => {});
    }

    stepIndex++;
  }, 400);

  activeMovements.set(movementKey, intervalId);
}

function stopDriverMovementAnimation(driverId) {
  if (!driverId) return;
  for (const [key, timerId] of activeMovements.entries()) {
    if (key.includes(driverId)) {
      clearInterval(timerId);
      activeMovements.delete(key);
    }
  }
}

async function addBookingToLayerGroup(booking, group, color) {
  const startIcon = L.divIcon({
    html: `<div style="color: ${color}; font-size: 28px; filter: drop-shadow(0 0 10px ${color});"><i class="fa-solid fa-location-dot"></i></div>`,
    className: "custom-leaflet-icon",
    iconSize: [28, 28],
    iconAnchor: [14, 28],
  });

  const startMarker = L.marker([booking.customerPos.lat, booking.customerPos.lng], { icon: startIcon });
  startMarker.bindPopup(`
    <b>Booking #${booking.id.slice(0, 8)}</b><br>
    Loại xe: ${booking.vehicleType || "Tất cả"}<br>
    Thanh toán: ${booking.paymentMethod || "CASH"} | Hạng: ${booking.customerTier || "REGULAR"}<br>
    Trạng thái: <b>${booking.status}</b>
  `);
  group.addLayer(startMarker);

  if (booking.status === "CANCELLED" || booking.status === "FAILED" || booking.status === "COMPLETED") {
    if (booking.driverId) {
      stopDriverMovementAnimation(booking.driverId);
    }
    return;
  }

  // ONLY draw route line & animate movement if booking is active (ASSIGNING or ACCEPTED)
  if ((booking.status === "ASSIGNING" || booking.status === "ACCEPTED") && booking.driverId && drivers.has(booking.driverId)) {
    const driver = drivers.get(booking.driverId);
    const lineColor = booking.status === "ACCEPTED" ? "#00f0ff" : "#f59e0b";

    // Fetch Real Road Navigation Path from OSRM Routing Engine
    const pathCoords = await fetchRealRoadPath(driver.position, booking.customerPos);

    const driverToPickupLine = L.polyline(pathCoords, {
      color: lineColor,
      weight: 5,
      opacity: 0.9,
      dashArray: "10, 10",
      className: "animated-route-line",
    });
    driverToPickupLine.bindTooltip(`🚗 Tuyến đường di chuyển đón khách (${booking.status})`, { sticky: true });
    group.addLayer(driverToPickupLine);

    // If ACCEPTED, animate driver vehicle moving along the real road path to pickup point!
    if (booking.status === "ACCEPTED") {
      startDriverMovementAnimation(booking.driverId, pathCoords, booking.id);
    }
  }
}

function removeBookingState(bookingId) {
  bookings.delete(bookingId);
  if (bookingMarkers.has(bookingId)) {
    map.removeLayer(bookingMarkers.get(bookingId));
    bookingMarkers.delete(bookingId);
  }
  requestRenderBookingsList();
}

function renderDriversListInternal() {
  const container = document.getElementById("drivers-list");
  document.getElementById("tab-driver-count").innerText = drivers.size;

  if (drivers.size === 0) {
    container.innerHTML = `<div class="empty-state">Chưa có tài xế nào. Bấm "+ Thêm Tài Xế" để tạo!</div>`;
    return;
  }

  let html = "";
  drivers.forEach((d) => {
    let badgeClass = "badge-idle";
    let isAssigning = d.status === "ASSIGNING";
    if (isAssigning) badgeClass = "badge-assigning";
    if (d.status === "BUSY") badgeClass = "badge-busy";

    const walletVal = d.walletBalance || 0;
    const walletText = walletVal.toLocaleString("vi-VN") + "đ";
    const walletColor = walletVal >= 20000 ? "text-green" : "text-red";

    const fatigueVal = d.drivingMinutes || 0;
    const fatigueColor = fatigueVal >= 240 ? "text-red" : (fatigueVal > 180 ? "text-yellow" : "text-muted");

    let actionsHtml = "";
    if (isAssigning && d.currentBookingId && bookings.has(d.currentBookingId)) {
      const b = bookings.get(d.currentBookingId);
      if (b.assignmentToken) {
        actionsHtml = `
          <div class="card-actions">
            <button class="btn btn-success btn-sm" onclick="respondBooking('${b.id}', '${d.id}', 'ACCEPT', '${b.assignmentToken}')">
              <i class="fa-solid fa-check"></i> CHẤP NHẬN
            </button>
            <button class="btn btn-danger btn-sm" onclick="respondBooking('${b.id}', '${d.id}', 'REJECT', '${b.assignmentToken}')">
              <i class="fa-solid fa-xmark"></i> TỪ CHỐI
            </button>
          </div>
        `;
      }
    }

    html += `
      <div class="item-card ${isAssigning ? 'item-assigning' : ''}">
        <div class="card-top">
          <span class="card-title">
            ${d.name} <small style="font-weight:normal; color:#94a3b8;">(${d.vehicleType || "Xe Máy"})</small>
            <button class="btn-edit-driver" onclick="openEditDriverModal('${d.id}')" title="Chỉnh sửa thông tin tài xế">
              <i class="fa-solid fa-pen-to-square"></i>
            </button>
          </span>
          <span class="card-badge ${badgeClass}">${d.status}</span>
        </div>
        
        <div class="card-metrics-grid">
          <div class="metric-tag">⭐ Rating: <strong>${d.rating || 4.8}</strong></div>
          <div class="metric-tag">🎯 Nhận đơn: <strong>${d.acceptanceRate || 95}%</strong></div>
          <div class="metric-tag">💰 Ví tiền: <strong class="${walletColor}">${walletText}</strong></div>
          <div class="metric-tag">⏱️ Lái xe: <strong class="${fatigueColor}">${fatigueVal}m/240m</strong></div>
          <div class="score-pill">⚡ Dispatch Score: ${d.score || 0} pts</div>
        </div>

        ${actionsHtml}
        
        <div class="card-sub">
          <span>Chế độ nhận đơn:</span>
          <select class="bot-select" onchange="changeBotMode('${d.id}', this.value)">
            <option value="MANUAL" ${d.autoBotMode === "MANUAL" ? "selected" : ""}>👨‍✈️ Thủ công (Bấm nút)</option>
            <option value="AUTO_ACCEPT" ${d.autoBotMode === "AUTO_ACCEPT" ? "selected" : ""}>⚡ Tự động nhận đơn</option>
          </select>
        </div>
      </div>
    `;
  });
  container.innerHTML = html;
}

function renderBookingsListInternal() {
  const container = document.getElementById("bookings-list");
  document.getElementById("tab-booking-count").innerText = bookings.size;

  if (bookings.size === 0) {
    container.innerHTML = `<div class="empty-state">Chưa có cuốc xe nào được khởi tạo</div>`;
    return;
  }

  let html = "";
  Array.from(bookings.values())
    .reverse()
    .slice(0, 20)
    .forEach((b) => {
      let badgeClass = "badge-pending";
      if (b.status === "ASSIGNING") badgeClass = "badge-assigning";
      if (b.status === "ACCEPTED") badgeClass = "badge-accepted";
      if (b.status === "FAILED") badgeClass = "badge-failed";
      if (b.status === "CANCELLED") badgeClass = "badge-cancelled";

      const driverName = b.driverId && drivers.has(b.driverId) ? drivers.get(b.driverId).name : "Chưa có";
      const excludedCount = b.excludedDriverIds ? b.excludedDriverIds.length : 0;

      const isVIP = b.customerTier === "VIP" || b.customerTier === "PLATINUM";
      const tierBadge = isVIP ? `<span style="color:#fbbf24; font-weight:700;">👑 ${b.customerTier}</span>` : `<span style="color:#94a3b8;">👤 ${b.customerTier || "REGULAR"}</span>`;
      const payBadge = b.paymentMethod === "CASH" ? "💵 Tiền mặt" : (b.paymentMethod === "CARD" ? "💳 Thẻ" : "📱 Ví e-Wallet");

      let actionBtns = "";
      if (b.status === "ACCEPTED" || b.status === "ASSIGNING") {
        actionBtns += `<button class="btn btn-success btn-sm" style="margin-right: 4px;" onclick="completeBooking('${b.id}')"><i class="fa-solid fa-flag-checkered"></i> Hoàn Thành</button>`;
      }
      if (b.status === "PENDING" || b.status === "ASSIGNING" || b.status === "ACCEPTED") {
        actionBtns += `<button class="btn btn-danger btn-sm" onclick="openCancelModal('${b.id}')"><i class="fa-solid fa-ban"></i> Hủy Cuốc</button>`;
      }

      html += `
        <div class="item-card">
          <div class="card-top">
            <span class="card-title">Booking #${b.id.slice(0, 8)} ${tierBadge}</span>
            <span class="card-badge ${badgeClass}">${b.status}</span>
          </div>
          
          <div class="card-metrics-grid">
            <div class="metric-tag">🚕 Xe: <strong>${b.vehicleType || "Tất cả"}</strong></div>
            <div class="metric-tag">💳 Cước: <strong>${payBadge}</strong></div>
            <div class="metric-tag" style="grid-column: span 2;">👨‍✈️ Tài xế: <strong>${driverName}</strong></div>
          </div>

          <div class="card-sub">
            <span>Từ chối: <strong style="color:#f43f5e;">${excludedCount} người</strong></span>
            <div>${actionBtns}</div>
          </div>
        </div>
      `;
    });

  container.innerHTML = html;
}

function completeBooking(bookingId) {
  fetch(`/api/bookings/${bookingId}/complete`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  })
    .then((res) => res.json())
    .then((data) => {
      if (data.success) {
        addLogLine(`🏁 [Hoàn Thành] Đã xác nhận hoàn thành cuốc xe #${bookingId.slice(0, 8)}!`, "success");
      }
    });
}

function updateStats(stats) {
  document.getElementById("stat-total-drivers").innerText = stats.totalDrivers;
  document.getElementById("stat-idle-drivers").innerText = stats.idleDrivers;
  document.getElementById("stat-busy-drivers").innerText = stats.busyDrivers;

  document.getElementById("stat-total-bookings").innerText = stats.totalBookings;
  document.getElementById("stat-pending-bookings").innerText = stats.pendingBookings;
  document.getElementById("stat-accepted-bookings").innerText = stats.acceptedBookings;
}

function addLogLine(msg, level = "info") {
  const consoleEl = document.getElementById("logs-console");
  const div = document.createElement("div");
  div.className = `log-line log-${level}`;
  div.innerText = msg;
  consoleEl.appendChild(div);
  consoleEl.scrollTop = consoleEl.scrollHeight;
}

function respondBooking(bookingId, driverId, action, token) {
  fetch(`/api/bookings/${bookingId}/respond`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ driverId, action, token }),
  });
}

function changeBotMode(driverId, mode) {
  fetch(`/api/drivers/${driverId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ autoBotMode: mode }),
  });
}

function openEditDriverModal(driverId) {
  const driver = drivers.get(driverId);
  if (!driver) return;

  document.getElementById("edit-driver-id").value = driver.id;
  document.getElementById("edit-driver-name").value = driver.name;
  document.getElementById("edit-driver-vehicle").value = driver.vehicleType || "Xe Máy 🛵";
  document.getElementById("edit-driver-rating").value = driver.rating || 4.8;
  document.getElementById("edit-driver-acceptance").value = driver.acceptanceRate || 95;
  document.getElementById("edit-driver-wallet").value = driver.walletBalance || 50000;
  document.getElementById("edit-driver-fatigue").value = driver.drivingMinutes || 60;
  document.getElementById("edit-driver-botmode").value = driver.autoBotMode || "MANUAL";

  document.getElementById("edit-driver-modal").classList.remove("hidden");
}

function updateCancelReasons(cancelledBy) {
  const select = document.getElementById("cancel-reason-select");
  if (!select) return;

  if (cancelledBy === "DRIVER") {
    select.innerHTML = `
      <option value="Tài xế gặp sự cố xe / Thủng lốp" selected>🛠️ Tài xế gặp sự cố xe / Thủng lốp</option>
      <option value="Kẹt xe nghiêm trọng không tới kịp ETA">🚦 Kẹt xe nghiêm trọng không tới kịp ETA</option>
      <option value="Khách gọi không nghe máy (No-Show)">📞 Khách gọi không nghe máy (No-Show)</option>
      <option value="Ngõ đón quá hẹp xe không vào được">🛣️ Ngõ đón quá hẹp xe không vào được</option>
    `;
  } else {
    select.innerHTML = `
      <option value="Khách đợi quá lâu / Đặt lại cuốc mới" selected>⏱️ Khách đợi quá lâu / Đặt lại (Cooldown 15p)</option>
      <option value="Tài xế đứng yên ngâm đơn">🛑 Tài xế đứng yên ngâm đơn</option>
      <option value="Thay đổi kế hoạch / Đổi lộ trình">🔄 Thay đổi kế hoạch / Đổi lộ trình</option>
      <option value="Đặt nhầm vị trí điểm đón">📍 Đặt nhầm vị trí điểm đón</option>
    `;
  }
}

function openCancelModal(bookingId) {
  document.getElementById("cancel-booking-id").value = bookingId;
  const cancelledBy = document.getElementById("cancel-by-user").value || "CUSTOMER";
  updateCancelReasons(cancelledBy);
  document.getElementById("cancel-booking-modal").classList.remove("hidden");
}

function initEventListeners() {
  // Sidebar tab switching
  document.querySelectorAll(".tab-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const targetTabId = btn.getAttribute("data-tab");
      if (!targetTabId) return;

      document.querySelectorAll(".tab-btn").forEach((b) => b.classList.remove("active"));
      document.querySelectorAll(".tab-content").forEach((c) => {
        c.classList.remove("active");
        c.classList.add("hidden");
      });

      btn.classList.add("active");
      const targetPane = document.getElementById(targetTabId);
      if (targetPane) {
        targetPane.classList.remove("hidden");
        targetPane.classList.add("active");
      }
    });
  });

  const btnClearLogs = document.getElementById("btn-clear-logs");
  if (btnClearLogs) {
    btnClearLogs.addEventListener("click", () => {
      const logs = document.getElementById("logs-console");
      if (logs) logs.innerHTML = "";
    });
  }

  // Dynamic cancel reasons when changing CancelledBy role
  const cancelBySelect = document.getElementById("cancel-by-user");
  if (cancelBySelect) {
    cancelBySelect.addEventListener("change", (e) => {
      updateCancelReasons(e.target.value);
    });
  }

  // Modal Cancel Booking Submit
  const btnSubmitCancel = document.getElementById("btn-submit-cancel");
  if (btnSubmitCancel) {
    btnSubmitCancel.addEventListener("click", () => {
      const bookingId = document.getElementById("cancel-booking-id")?.value;
      const cancelledBy = document.getElementById("cancel-by-user")?.value;
      const reason = document.getElementById("cancel-reason-select")?.value;

      if (!bookingId) return;

      fetch(`/api/bookings/${bookingId}/cancel`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cancelledBy, reason }),
      })
        .then((res) => res.json())
        .then(() => {
          addLogLine(`🚫 [API Cancel] Hủy cuốc ${bookingId} thành công (Kích hoạt Cooldown 15p Redis)!`, "warn");
          document.getElementById("cancel-booking-modal")?.classList.add("hidden");
        });
    });
  }

  document.getElementById("btn-close-cancel-modal")?.addEventListener("click", () => {
    document.getElementById("cancel-booking-modal")?.classList.add("hidden");
  });
  document.getElementById("btn-dismiss-cancel-modal")?.addEventListener("click", () => {
    document.getElementById("cancel-booking-modal")?.classList.add("hidden");
  });

  // Confirm Pickup Booking
  document.getElementById("btn-confirm-booking")?.addEventListener("click", () => {
    if (!tempPickupCoords) return;
    const vehicleType = document.getElementById("pickup-vehicle-type")?.value || document.getElementById("select-vehicle-type")?.value || "Tất cả";
    const paymentMethod = document.getElementById("select-payment-method")?.value || "CASH";
    const customerTier = document.getElementById("select-customer-tier")?.value || "REGULAR";

    fetch("/api/bookings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        customerPos: tempPickupCoords,
        vehicleType: vehicleType,
        paymentMethod: paymentMethod,
        customerTier: customerTier,
      }),
    })
      .then((res) => res.json())
      .then((data) => {
        addLogLine(`🚀 [Booking Created] Tạo cuốc mới #${data.id.slice(0, 8)} (${paymentMethod} | ${customerTier}) thành công!`, "info");
        clearTempPickup();
      });
  });

  document.getElementById("btn-cancel-pickup")?.addEventListener("click", () => {
    clearTempPickup();
  });

  // Edit Driver Save Button
  const btnSaveDriver = document.getElementById("btn-save-driver");
  if (btnSaveDriver) {
    btnSaveDriver.addEventListener("click", () => {
      const id = document.getElementById("edit-driver-id")?.value;
      const name = document.getElementById("edit-driver-name")?.value;
      const vehicle = document.getElementById("edit-driver-vehicle")?.value;
      const rating = parseFloat(document.getElementById("edit-driver-rating")?.value || "5.0");
      const acceptance = parseFloat(document.getElementById("edit-driver-acceptance")?.value || "100");
      const wallet = parseInt(document.getElementById("edit-driver-wallet")?.value || "0");
      const fatigue = parseInt(document.getElementById("edit-driver-fatigue")?.value || "0");
      const mode = document.getElementById("edit-driver-botmode")?.value || "MANUAL";

      if (!id) return;

      fetch(`/api/drivers/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name,
          vehicleType: vehicle,
          rating: rating,
          acceptanceRate: acceptance,
          walletBalance: wallet,
          drivingMinutes: fatigue,
          autoBotMode: mode,
        }),
      }).then(() => {
        document.getElementById("edit-driver-modal")?.classList.add("hidden");
      });
    });
  }

  document.getElementById("btn-close-edit-modal")?.addEventListener("click", () => {
    document.getElementById("edit-driver-modal")?.classList.add("hidden");
  });
  document.getElementById("btn-cancel-edit-modal")?.addEventListener("click", () => {
    document.getElementById("edit-driver-modal")?.classList.add("hidden");
  });

  // Add 5 Drivers Quick Button
  document.getElementById("btn-add-driver")?.addEventListener("click", () => {
    for (let i = 0; i < 5; i++) {
      fetch("/api/drivers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      })
        .then((res) => res.json())
        .then((driver) => {
          if (driver && driver.id) {
            updateDriverState(driver);
          }
        });
    }
    addLogLine("🚕 [System] Đã nạp thành công 5 xe tài xế mới lên bản đồ!", "info");
  });

  const btnStressTest = document.getElementById("btn-stress-test");
  if (btnStressTest) {
    btnStressTest.addEventListener("click", () => {
      fetch("/api/simulation/stress-test?drivers=5&bookings=5", { method: "POST" })
        .then((res) => res.json())
        .then(() => {
          addLogLine("⚡ [Stress Test] Đã kích hoạt bão cuốc thử nghiệm!", "warn");
        });
    });
  }
}

function openAdminModal() {
  fetchInfraStatus();
  const modal = document.getElementById("admin-modal");
  if (modal) modal.classList.remove("hidden");
}

function closeAdminModal() {
  const modal = document.getElementById("admin-modal");
  if (modal) modal.classList.add("hidden");
}

function adminDepositAll() {
  fetch("/api/admin/drivers/deposit-all", { method: "POST" })
    .then((res) => res.json())
    .then(() => {
      addLogLine("💰 [Admin] Đã cộng 100,000đ vào ví cho tất cả tài xế!", "success");
    });
}

function adminResetFatigue() {
  fetch("/api/admin/drivers/reset-fatigue", { method: "POST" })
    .then((res) => res.json())
    .then(() => {
      addLogLine("⏱️ [Admin] Đã reset thời gian mệt mỏi về 0 phút cho tất cả tài xế!", "success");
    });
}

function adminAutoAcceptAll() {
  fetch("/api/admin/drivers/auto-accept-all", { method: "POST" })
    .then((res) => res.json())
    .then(() => {
      addLogLine("⚡ [Admin] Đã bật chế độ Tự Động Nhận Đơn cho toàn bộ tài xế!", "info");
    });
}

function adminClearCooldowns() {
  fetch("/api/admin/clear-cooldowns", { method: "POST" })
    .then((res) => res.json())
    .then((data) => {
      addLogLine(`🔒 [Admin] Đã mở khóa ${data.clearedKeys || 0} Redis Cooldown keys!`, "success");
      fetchInfraStatus();
    });
}

function adminClearBookings() {
  fetch("/api/admin/clear-bookings", { method: "POST" })
    .then((res) => res.json())
    .then(() => {
      bookings.clear();
      bookingMarkers.forEach((g) => map.removeLayer(g));
      bookingMarkers.clear();
      addLogLine("🧹 [Admin] Đã dọn dẹp sạch toàn bộ cuốc xe cũ!", "info");
      requestRenderBookingsList();
      fetchInfraStatus();
    });
}

function fetchInfraStatus() {
  fetch("/api/admin/infra-status")
    .then((res) => res.json())
    .then((data) => {
      document.getElementById("admin-pg-status").innerText = data.postgresStatus ? "🟢 ONLINE (Outbox Table Active)" : "🔴 OFFLINE (In-Memory Fallback)";
      document.getElementById("admin-outbox-count").innerText = data.outboxEvents || 0;
      
      document.getElementById("admin-redis-status").innerText = data.redisStatus ? "🟢 ONLINE (SETNX Lock Active)" : "🔴 OFFLINE (In-Memory Lock)";
      document.getElementById("admin-cooldown-count").innerText = data.cooldownKeys || 0;

      document.getElementById("admin-mq-status").innerText = data.rabbitmqStatus ? "🟢 ONLINE (Queue Active)" : "🔴 OFFLINE (Direct Channel)";
      document.getElementById("admin-goroutines-count").innerText = data.goroutines || 0;
    })
    .catch(() => {});
}
