# BSM Driver Scoring Algorithm — Principal Engineer Review

**Reviewing:** `algo.md` — BSM Driver Scoring Algorithm Catalog v1.0
**Scope of this review:** Algorithm design only — metrics, scoring model, candidate filtering, matching algorithms, routing, algorithm-level fallback, and performance *assumptions*. Infrastructure topics already present in the source document (§2.4: Redis sharding, Go worker pools, lock-free indexing) are explicitly **out of scope** and left untouched, per the review brief.

**Note on numbering:** The source document already contains its own issue markers (`#1`–`#20`) from a prior review round. To avoid collisions, every finding below is labeled **F1–F15** and is independent of that earlier numbering.

All quotes are reproduced verbatim from `algo.md`. All revisions are minimal, surgical edits — no new features, metrics, or infrastructure are introduced.

---

## Summary Table

| # | Section | Severity | One-line issue |
|---|---------|----------|-----------------|
| F1 | 1.1 | Medium | $B_{barrier}$ has no defined numeric range, so the $G_{barrier}$ floor (0.4) and coefficient (0.20) are unverifiable |
| F2 | 1.2 | **Critical** | $AR$ / $CoR$ are defined as raw ratios but every formula divides them by 100 — scale is never stated |
| F3 | 1.2 | Low | $F_{penalty}$ is defined but never wired into any filter or formula (orphaned variable) |
| F4 | 1.4 | Medium | $C_{vip}$ is catalogued as a metric but the $S_{VIP}$ formula never references it — hardcodes `10.0` instead |
| F5 | 1.5 / 2.1 | High | $P_{revenue}$ formula contradicts its own worked example, and its stated cap (5.0) is mathematically unreachable |
| F6 | 2.1 | **Critical** | $S_{idle\_fifo}$ is used in the $S_{boost}$ sum but is never defined anywhere in the document |
| F7 | 2.3.B | **Critical** | The batch "Cost" matrix is named/framed for *minimization* but the design requires *maximization* — a literal implementation inverts the ranking |
| F8 | 2.3.B | Low | $AR$ silently influences the batch ranking twice (once in `Score`, once in `P_accept`) — needs an explicit design note, not a math fix |
| F9 | 2.3.C | High | "Greedy Single-Assignment" is labeled $O(1)$ in a context where it replaces a whole-batch solver — should be $O(V)$ there |
| F10 | 2.5.A | Medium | $L_{cash}$ is listed as an active priority signal in the decision matrix despite being tagged Phase 2 / inactive in v1.0 |
| F11 | 2.5.A | High | No precedence rule exists for when multiple regime rows' conditions are simultaneously true |
| F12 | 2.5.B | High | Hysteresis Guard only covers 2 of the 3 regime-transition boundaries (0.8, 1.5) and omits 3.0 |
| F13 | 2.5.C | Medium | Benchmark item #6 calls `MinScore = 60.0` the "floor," contradicting §3.2's actual floor of 30.0 |
| F14 | 3.1 | Medium | Tie-break Priority 3 uses wallet balance ($L_{cash}$), a metric explicitly inactive in v1.0 |
| F15 | 3.2 | High | The MinScore decay sequence is only illustrated for attempts 0–2; no closed-form (with floor clamp) is given, leaving attempts 3–5 ambiguous |

Sections/subsections reviewed and found **sound as written** (no change recommended): 1.1's $\theta_{heading}$ design, 1.3, 2.1's weight-sum invariant and $\alpha_{ETA}$ constants, 2.2's latency waterfall arithmetic, 2.3.A candidate generation, 2.3.C Hungarian complexity derivation, 2.4 (out of scope), 2.5.B's Timeout Budget Guard and Rejection Loop rows, 2.5.C items 1–5, 3.3, and the Phase-2 RL reward function.

---

## Section 1.1 — Spatial & Temporal Metrics

### F1. $B_{barrier}$ has no defined range (Medium)

> **Original:** *"Chỉ Số Rào Cản Vật Lý ($B_{barrier}$): Chỉ số trở ngại di chuyển & khó khăn vận hành độc lập ... — đây là chi phí trải nghiệm UX không đo bằng khoảng cách/thời gian OSRM."*

**Why problematic:** §2.1 consumes this variable in $G_{barrier} = \max(0.4,\ 1 - 0.20 \cdot B_{barrier})$, but $B_{barrier}$'s scale is never stated. This isn't cosmetic — the 0.4 floor is only meaningful for a specific input range:
- If $B_{barrier} \in [0,1]$ (normalized), the floor is **unreachable** ($1 - 0.20 \times 1 = 0.8 > 0.4$) — dead code, same failure mode as F5 below.
- If $B_{barrier} \in [0,5]$, the floor binds exactly at $B_{barrier}=3$ and clamps beyond it — a sensible, reachable design.

An implementer cannot build the candidate-filtering/scoring pipeline without this decision being made explicit in the spec.

**Revised:**
> *"Chỉ Số Rào Cản Vật Lý ($B_{barrier}$): thang đo số nguyên $B_{barrier} \in [0, 5]$ (0 = không rào cản, 1–2 = nhẹ, 3 = trung bình, 4–5 = nghiêm trọng — hẻm nhỏ không quay đầu, khu đô thị khép kín có bảo vệ, rào chắn tạm thời). Với hệ số $w_{barrier}=0.20$ (BSM Car), sàn $G_{barrier}=0.4$ đạt được chính xác tại $B_{barrier}=3$ và giữ nguyên cho $B_{barrier} > 3$."*

**Why better:** Makes the 0.4 floor a reachable, intentional design point instead of an ambiguous (possibly dead) constant, and gives ops/data teams a concrete labeling scale to populate the field.

No other change needed in §1.1 — the $\theta_{heading}$ entry already correctly explains *why* it avoids double-counting (bearing passed into OSRM rather than scored separately), which is exactly the kind of self-documentation this review is asking for elsewhere.

---

## Section 1.2 — Driver Profile & Quality Metrics

### F2. $AR$ / $CoR$ scale is never stated, and the given definition contradicts the formulas that consume it (Critical)

> **Original:** *"Tỷ Lệ Nhận Đơn (AR - Acceptance Rate): Số đơn tài xế nhận chia cho số đơn hệ thống bắn xuống trong 24h."*
> *"Tỷ Lệ Hoàn Thành Cuốc (CoR - Completion Rate): Số đơn hoàn thành trên tổng số đơn đã nhận."*

**Why problematic:** Both definitions describe a ratio of counts — mathematically a fraction in $[0,1]$. But every formula that consumes these variables divides by 100: $w_2 \cdot (AR/100)$, $w_3 \cdot (CoR/100)$ in the core score, and $P_{accept} = (AR/100) \cdot e^{-0.002 t_{ETA}}$. If an engineer implements $AR$ exactly as defined here (e.g., `0.85`), dividing by 100 collapses it to `0.0085`, silently destroying $w_2$'s entire contribution to the score. This is precisely the kind of unit ambiguity that produces a passing code review and a broken production ranking.

**Revised:**
> *"Tỷ Lệ Nhận Đơn (AR): biểu diễn dưới dạng phần trăm trên thang đo 0–100: $AR = \dfrac{\text{số đơn nhận}}{\text{số đơn bắn xuống}} \times 100$, tính trong cửa sổ 24h."*
> *"Tỷ Lệ Hoàn Thành Cuốc (CoR): biểu diễn dưới dạng phần trăm trên thang đo 0–100: $CoR = \dfrac{\text{số đơn hoàn thành}}{\text{số đơn đã nhận}} \times 100$."*

**Why better:** Directly matches every downstream formula's `/100` convention, removing the single largest silent-failure risk in the document.

### F3. $F_{penalty}$ is defined but never used (Low)

> **Original:** *"Cảnh Báo Gần Đây ($F_{penalty}$): Số lượng vi phạm bị ghi nhận trong 24h."*

**Why problematic:** This metric is catalogued as an input dimension but does not appear in the scoring formula (§2.1), the candidate filter (§2.2 Stage 1 only filters `IDLE` state), or anywhere else. Meanwhile §2.5.B / §3.3 later introduce two granular counters, $F_{reject}$ and $F_{timeout}$, that cover the same conceptual ground (recent violations) but are never tied back to $F_{penalty}$. A reviewer can't tell if these are the same tracking system described at two levels of detail, or two independent, competing systems.

**Revised:**
> *"Cảnh Báo Gần Đây ($F_{penalty}$): Tổng số vi phạm ghi nhận trong 24h, $F_{penalty} = F_{reject} + F_{timeout}$ (định nghĩa chi tiết từng loại vi phạm và cơ chế cooldown tương ứng tại Mục 2.5.B / 3.3). Hiện $F_{penalty}$ chỉ mang tính thống kê/quan sát trong v1.0 và chưa được dùng làm điều kiện lọc cứng."*

**Why better:** Closes the loop between the catalog entry and its later, more detailed implementation, and is explicit that it's not (yet) wired into filtering — so nobody accidentally assumes it's an active gate.

No other change needed in §1.2 — $R_{star}$'s $[1.0, 5.0]$ range is well-defined and consistently used.

---

## Section 1.4 — System State & Order Metrics

### F4. $C_{vip}$ is catalogued as a metric but never appears in the formula that should use it (Medium)

> **Original (§1.4):** *"Hạng Khách Hàng ($C_{vip}$): Điểm ưu tiên nếu khách hàng thuộc nhóm VIP/Platinum."*
> **Original (§2.1, #6):** *"$S_{VIP}(\text{order\_attempt}) = 10.0 \times 0.8^{\text{order\_attempt}}$"*

**Why problematic:** §1.4 promises $C_{vip}$ is a "priority score" — implying it can vary (e.g., different values for VIP vs. Platinum). But the $S_{VIP}$ formula that's supposed to consume it hardcodes the constant `10.0` and never references the variable $C_{vip}$ at all. Either the metric catalog is overpromising a graded signal that doesn't exist, or the formula is silently dropping an input it should be using. As written, the two sections are simply inconsistent about whether $C_{vip}$ is a real input or a documentation-only placeholder.

**Revised:**
> $$S_{VIP}(\text{order\_attempt}) = C_{vip} \times 0.8^{\text{order\_attempt}}$$
> *"với $C_{vip} \in \{0.0,\ 10.0\}$ trong v1.0 (0.0 = khách thường, 10.0 = khách VIP/Platinum theo phân loại tại Mục 1.4). Cấu trúc nhân này cho phép mở rộng $C_{vip}$ thành thang điểm nhiều bậc (VIP vs. Platinum) trong tương lai mà không cần đổi công thức decay."*

**Why better:** Makes the catalog entry and the formula agree, preserves today's exact numeric behavior (10.0 for VIP, so no regression), and makes the formula genuinely extensible if VIP tiers are later graded — which is what §1.4's own wording implied it should already support.

No other change needed in §1.4 — $t_{wait}$, $S_{D,ratio}$, and $V_{fare}$ are clearly defined and consistently used downstream.

---

## Sections 1.5 / 2.1 (#7) — $P_{revenue}$ Formula

### F5. The $P_{revenue}$ formula contradicts its own worked example, and its cap is unreachable (High)

> **Original (§1.5):** *"$P_{revenue} = \min(5.0,\ \text{FareRatio} \times CoR)$"*
> **Original (§2.1, #7 worked example):** *"Với tài xế có $CoR = 90\% (0.9)$, điểm $P_{revenue} = \min(5.0, 3.0 \times 0.9) = 2.7$ điểm."*

**Why problematic:** Two separate bugs stack here:
1. **Unit contradiction.** The formula's own worked example treats $CoR$ as a 0–1 fraction (`0.9`), but F2 above establishes — and every other formula in the document confirms — that $CoR$ is stored on a 0–100 scale. If $CoR$ is actually `90` (per its correct definition) and plugged directly into $\text{FareRatio} \times CoR$ as literally written, the result is $3.0 \times 90 = 270$, not $2.7$. The formula as written and its own example only agree by accident, because the example silently applies a `/100` the formula text never shows.
2. **Dead ceiling.** $\text{FareRatio}$ is itself capped at $3.0$ (§2.1 #7), and $CoR/100 \le 1.0$ by definition. So the true maximum of $\text{FareRatio} \times (CoR/100)$ is $3.0 \times 1.0 = 3.0$ — the stated cap of $5.0$ can never be reached. It reads as an intentional safety ceiling but is actually unreachable dead code, which misleads anyone trying to reason about $S_{boost}$'s true maximum (see F6).

**Revised:**
> $$P_{revenue} = \min\left(3.0,\ \text{FareRatio} \times \frac{CoR}{100}\right)$$
> *"Ví dụ số: ... Với tài xế có $CoR = 90$ (thang 0–100), điểm $P_{revenue} = \min(3.0,\ 3.0 \times 0.9) = 2.7$ điểm."*

**Why better:** The formula now matches its own worked example exactly (same $2.7$ result — no behavior change for this example), the units are consistent with every other use of $CoR$ in the document, and the cap ($3.0$) is the true, reachable maximum rather than a misleading unreachable one. This also gives F6 below an accurate number to build on.

---

## Section 2.1 — Non-linear Reciprocal Decay Core Model

### F6. $S_{idle\_fifo}$ is used but never defined (Critical)

> **Original:** *"Tổng trần $S_{boost} \le 30.0$: $S_{boost} = S_{aging} + S_{VIP} + S_{idle\_fifo} + P_{revenue}$. Tổng điểm cực đại $Score_{total} \le 130.0$."*

**Why problematic:** $S_{aging}$ has an explicit saturating formula (max $10.0$), $S_{VIP}$ has an explicit formula (max $10.0$ at `order_attempt=0`), and $P_{revenue}$ now has a corrected max of $3.0$ (F5). But **$S_{idle\_fifo}$ has no formula anywhere in the document** — it's only named in §1.3 as "$t_{idle}$ ... Dùng cho công bằng FIFO" (a metric, not a scoring function) and then appears unexplained inside the $S_{boost}$ sum. Without a defined formula and bound, the stated invariants "$S_{boost} \le 30.0$" and "$Score_{total} \le 130.0$" are unverifiable — and if an implementer invents their own unbounded version of $S_{idle\_fifo}$, both invariants can silently break, along with the $MinScore$/`Score_total` relationship the whole matching pipeline depends on.

**Revised:** Define it with the same saturating-exponential shape already used for $S_{aging}$ (consistent design language), sized so the total budget still sums to exactly $30.0$ given the corrected component maximums ($10.0 + 10.0 + 3.0 = 23.0$, leaving $7.0$):
> $$S_{idle\_fifo} = \min\left(7.0,\ 7.0 \times \left(1 - e^{-\beta \cdot t_{idle}}\right)\right)$$
> *"với $\beta$ là hằng số làm mịn cần benchmark thực tế (tương tự cách $S_{aging}$ dùng $0.005$ tại Mục 2.5.C), giá trị đề xuất khởi điểm $\beta = 0.001$. Với các cận trên $S_{aging} \le 10.0$, $S_{VIP} \le 10.0$, $S_{idle\_fifo} \le 7.0$, $P_{revenue} \le 3.0$ (đã sửa tại Mục 1.5), tổng $S_{boost} \le 30.0$ và $Score_{total} \le 130.0$ được đảm bảo đúng như công bố."*

Also recommend adding this to the config-load invariant check already specified for $w_1+w_2+w_3=1.0$ (§2.1): assert at startup that the sum of each boost component's hard-coded max equals the documented $30.0$ ceiling, the same way the weight-sum invariant is enforced.

**Why better:** Closes the single largest gap in the scoring model — a component that's summed into a hard business invariant but was otherwise undefined. The proposed shape is not a new design; it mirrors $S_{aging}$'s existing pattern, keeping the document internally consistent rather than introducing a new mechanism.

### Minor note (bundled with F6, not a separate finding)
The $S_{aging}$ worked example computes $t=60\text{s} \Rightarrow +2.5$ points, but $10 \times (1 - e^{-0.005 \times 60}) = 2.59$, not $2.5$. The $3$-minute ($5.93 \approx 5.9$) and $10$-minute ($9.50 \approx 9.5$) examples check out exactly. Recommend correcting the first example to "**+2.6 điểm**" for consistency — trivial, but worth a one-character fix since the other two examples are held to two-significant-figure accuracy.

No other change needed in §2.1 — the $w_1+w_2+w_3=1.0$ invariant, the default weight configs (both sum to 1.0 correctly), and the $\alpha_{ETA}$ decay constants are internally consistent and appropriately deferred to benchmarking (§2.5.C) rather than hard-coded as unverified truth.

---

## Section 2.3.B — Batch Acceptance Protocol

### F7. The "Cost" matrix is framed for minimization, but the design requires maximization (Critical)

> **Original:** *"Nhúng Xác suất Chấp nhận ($P_{accept}$) vào Ma trận Chi phí: $\text{Cost}(o_i, d_j) = \text{Score}(o_i, d_j) \cdot P_{accept}(AR, t_{ETA})$ ... Ma trận chi phí chủ động ưu tiên các tài xế có xác suất nhận cuốc cao."*

**Why problematic:** This is an algorithm-correctness bug, not a style issue. The classical Hungarian algorithm — which §2.3.C explicitly specifies as the solver for $V \le 30$ — **minimizes** total cost by convention, and most off-the-shelf implementations (and most engineers' mental model of "Cost") default to minimization. But here, `Score` is a *reward* (higher = better driver) and `P_accept` is a *probability* (higher = more likely to accept) — both terms are things the system wants to **maximize**, and the very next sentence confirms the intent is to "prioritize" (favor) high-value pairs. If this matrix is handed to a standard minimizing Hungarian solver exactly as named, the solver will preferentially select the *worst*-scoring, *least*-likely-to-accept pairs — the exact opposite of the stated goal, and a bug that would be very easy to ship because the code would run without error, just against inverted rankings.

**Revised:**
> *"Nhúng Xác suất Chấp nhận ($P_{accept}$) vào Ma trận Trọng số (Weight Matrix):* $$\text{Weight}(o_i, d_j) = \text{Score}(o_i, d_j) \cdot P_{accept}(AR, t_{ETA})$$ *với $P_{accept} = (AR/100) \cdot e^{-0.002 \cdot t_{ETA}}$. Ma trận này được đưa vào Hungarian/Auction Algorithm dưới dạng bài toán **tối đa hóa (maximization)** tổng trọng số toàn cục. Nếu thư viện solver chỉ hỗ trợ minimization theo quy ước cổ điển, bắt buộc áp dụng phép biến đổi $\text{Cost}(o_i,d_j) = C_{max} - \text{Weight}(o_i,d_j)$ trước khi đưa vào solver, với $C_{max}$ là hằng số $\ge$ giá trị Weight lớn nhất có thể (ví dụ $C_{max}=130$, khớp trần $Score_{total} \le 130.0$ tại Mục 2.1)."*

**Why better:** Removes the single most implementation-dangerous ambiguity in the document: it explicitly names the optimization direction and gives the exact transformation needed if the chosen solver library doesn't natively support maximization, rather than leaving direction-of-optimization as an implicit assumption.

### F8. $AR$ influences the batch ranking twice — needs a design note, not a formula change (Low)

**Why problematic:** $AR$ already contributes to `Score` via $w_2 \cdot (AR/100)$ in the core formula (§2.1), and then contributes again, at full weight, via $P_{accept} = (AR/100) \cdot e^{-0.002 t_{ETA}}$ in the batch Weight matrix (F7). This means the *same* driver's $AR$ compounds twice in batch mode but only counts once in single-order Greedy mode (§2.5.A rows 1–2), so the same driver profile can rank differently between the two dispatch paths purely because of this asymmetry.

This is plausibly *intentional* — using $AR$ both as a general quality signal and as an acceptance-probability proxy is a defensible modeling choice specifically for the "one rejection collapses the global optimum" problem §2.3.B is solving — but the document doesn't say so, which risks a future engineer "fixing" it by removing one instance.

**Revised:** Add a one-line rationale rather than change the math:
> *"Lưu ý thiết kế: $AR$ được sử dụng có chủ đích ở cả hai vai trò — tín hiệu chất lượng tổng quát trong $Score$ (trọng số $w_2$) và ước lượng xác suất chấp nhận tức thời trong $P_{accept}$ — nhằm chủ động thiên vị batch-matching về phía các tài xế ít có khả năng từ chối. Đây không phải lỗi double-counting; không loại bỏ một trong hai instance khi refactor."*

**Why better:** Prevents an unnecessary "bug fix" that would remove a deliberate anti-rejection-cascade mechanism, while making the reviewer (or the next engineer) aware of the asymmetry between batch and non-batch ranking behavior.

---

## Section 2.3.C — Complexity & Router Rules

### F9. "Greedy Single-Assignment" is labeled $O(1)$ where it actually replaces a whole-batch solve — should be $O(V)$ (High)

> **Original:** *"$V > 200$ hoặc Solver Compute $> 10\text{ms}$: Kích hoạt Timeout Budget Guard, ngắt solver và hạ cấp về **Greedy Single-Assignment ($O(1)$)**."*

**Why problematic:** This entry sits in a table alongside Hungarian ($O(V^3)$) and Auction ($O(V^2/\epsilon)$) — both *whole-batch* complexities describing the cost of solving the full $V$-sized assignment problem. When the router downgrades to Greedy here, it must still process every one of the $V$ unmatched order/driver pairs left over from the aborted solve (§2.5.B: *"hạ cấp về Greedy Single-Assignment / Nearest-Neighbor **cho các đơn còn lại**"* — "for the **remaining orders**," plural). Each individual pick is $O(1)$ (or $O(K)$ for a small fixed candidate list $K$), but the *batch* fallback as a whole is $O(V)$, not $O(1)$. Labeling it $O(1)$ at the batch level is a genuine complexity-analysis error, not just imprecise phrasing — it would lead someone to (wrongly) assume the fallback path has no scaling cost at all.

Note this is a *different* usage than §2.5.A rows 🟢/🔵, where "Greedy Single-Assignment (O(1))" correctly describes picking one best driver for **one** incoming order from a fixed $K=20$ candidate list — that usage is genuinely $O(1)$ per order and needs no change.

**Revised:**
> *"$V > 200$ hoặc Solver Compute $> 10\text{ms}$: ... hạ cấp về **Greedy Single-Assignment ($O(V)$ cho toàn batch, $O(1)$ mỗi cặp gán)**."*

And in §2.5.B's guard row, append: *"(độ phức tạp $O(V)$ cho tổng số đơn còn lại, mỗi lần gán $O(1)$)."*

**Why better:** Correctly distinguishes per-item complexity from whole-batch complexity — the same distinction the document already correctly makes for Hungarian and Auction — and avoids the misleading implication that the fallback path is free regardless of how many orders are left when the timeout fires.

No other change needed in §2.3 — the Hungarian complexity derivation ($O(\max(M,N)^3)$, the $5^3=125$ vs. $(M+N)^3=1000$ example, and the resulting $8\times$ overestimate figure) is arithmetically correct and a genuinely useful correction of a common mistake; and the candidate-generation pipeline (top $K=5$ per order, single $M \times N$ OSRM Table call) is internally consistent.

---

## Section 2.5.A — Dynamic Dispatch Strategy Decision Matrix

### F10. $L_{cash}$ is listed as an active priority signal despite being tagged Phase 2 (Medium)

> **Original (🌧️ row, "Trọng số & Chỉ số Ưu tiên" column):** *"$C_{vip}$, $L_{cash}$, $P_{revenue}$, Surge"*
> **Contradicts (§1.5):** *"Tính thanh khoản hoa hồng lập tức ($L_{cash}$): *(Phase 2 — Chưa active trong v1.0)* Số dư ví tài khoản tài xế tại `account-svc`."*

**Why problematic:** The metrics catalog explicitly marks $L_{cash}$ as not active in v1.0 specifically to avoid confusing implementers (per the catalog's own framing note in §1). Listing it as a live priority signal for the "Heavy Rain / Flood / Tet" regime directly contradicts that. An engineer implementing this row would either wire up a Phase-2-only feature ahead of schedule, or silently drop it and wonder why the row doesn't match the formula.

**Revised:** Remove $L_{cash}$ from the row's active priority list (or mark it inline as deferred):
> *"$C_{vip}$, $P_{revenue}$, Surge *(ghi chú: $L_{cash}$ dự kiến bổ sung khi Phase 2 kích hoạt)*"*

**Why better:** Makes the decision matrix consistent with the metrics catalog's own activation status, so v1.0 implementers aren't asked to build against a feature the same document says isn't ready.

### F11. No precedence rule when multiple regime rows' conditions overlap (High)

> **Original:** Rows 1–4 classify by $S_{D,ratio}$ (ranges $<0.8$, $[0.8,1.5)$, $[1.5,3.0]$, $>3.0$ — this partition is internally consistent, no gaps or overlaps). Row 5 classifies by a *different* dimension ("Mật độ $<2$ xe/ô H3"). Row 6 applies to *any* ratio ("Mọi tỷ lệ").

**Why problematic:** Rows 1–4 are a clean, non-overlapping partition of $S_{D,ratio}$ — verified: $(-\infty,0.8)$, $[0.8,1.5)$, $[1.5,3.0]$, $(3.0,\infty)$ tile the real line exactly. But rows 5 and 6 are classified on entirely different axes (local H3-cell density; VIP order presence) and can be simultaneously true alongside any of rows 1–4. For example: a downtown zone (row 3: 🔴, ratio 2.0) could simultaneously have a sparse H3 sub-cell (row 5: 🟡) and contain a VIP order (row 6: ⚡). The document never states which strategy wins, which is a real production hazard — the router needs a deterministic answer, not an implicit one.

**Revised:** Add an explicit precedence rule immediately below the table:
> *"**Thứ tự ưu tiên khi nhiều điều kiện đồng thời đúng (Regime Precedence Order):**
> 1. ⚡ **VIP / Đơn giá trị cao** — kiểm tra trước tiên, độc lập với $S_{D,ratio}$; nếu thỏa, áp dụng Business Revenue Gate bất kể chế độ cung/cầu.
> 2. 🌧️ **Sự kiện khẩn cấp vùng ($S_{D,ratio} > 3.0$)** — override tiếp theo, do đây là tín hiệu toàn vùng.
> 3. **Chế độ theo $S_{D,ratio}$ (🟢/🔵/🔴)** — áp dụng nếu không rơi vào (1) hoặc (2).
> 4. 🟡 **Mật độ H3 cục bộ $<2$ xe/ô** — KHÔNG phải nhánh loại trừ lẫn nhau với (3); là một điều chỉnh cục bộ (k-ring expansion) lồng bên trong bất kỳ chế độ nào đã chọn ở bước 1–3, kích hoạt khi mật độ tuyệt đối trong một ô H3 cụ thể quá thấp."*

**Why better:** Converts an implicit, undocumented assumption into a deterministic rule the router can actually implement, and correctly reflects that row 5 is a *local modifier* rather than a mutually-exclusive regime like rows 1–4 and 6.

No other change needed in §2.5.A — the ratio partition across rows 1–4 is mathematically clean, and the Phase-2-labeled RL/bandit reward function at the bottom is correctly scoped as future work and requires no correction (its terms — match reward, completion, fare ratio, ETA penalty, zone Gini penalty — are individually well-formed).

---

## Section 2.5.B — Guards & Fallback Protocols

### F12. Hysteresis Guard omits the third regime boundary (High)

> **Original:** *"Ratio Cung/Cầu dao động quanh ranh giới (**0.8 hoặc 1.5**) → Áp dụng vạch trễ (Hysteresis Band ±0.15)..."*

**Why problematic:** The decision matrix (§2.5.A) has **three** regime-transition boundaries, not two: $0.8$ (🟢→🔵), $1.5$ (🔵→🔴), and $3.0$ (🔴→🌧️). The Hysteresis Guard — whose entire purpose is preventing regime flapping at transition boundaries — only explicitly covers two of the three. A demand spike hovering around ratio $3.0$ (exactly the "downtown becomes an emergency" transition, arguably the highest-stakes one, since it swaps the algorithm from Localized H3 Matching to Adaptive Batch Matching) would flap unprotected between 🔴 and 🌧️ strategies.

**Revised:**
> *"Ratio Cung/Cầu dao động quanh **bất kỳ ranh giới chuyển pha nào giữa 4 vùng chế độ (0.8, 1.5, hoặc 3.0)** → Áp dụng vạch trễ (Hysteresis Band ±0.15) kết hợp EMA (chu kỳ 30s) tại **cả ba** ranh giới trước khi chuyển kịch bản."*

**Why better:** Closes the gap for exactly the boundary most likely to cause visible thrashing (switching solver strategy entirely, not just a weight adjustment), using the same mechanism already specified for the other two boundaries — no new machinery introduced.

No other change needed in §2.5.B — the Timeout Budget Guard (10ms, consistent with §2.2 Stage 4 and §2.3.C), the Empty Candidate Pool relaxation sequence ($4.8 \to 4.5 \to 4.0$, consistent with §2.5.A row ⚡), and the Rejection Loop's `REJECT`/`TIMEOUT` split (consistent with §3.3) are all internally consistent as written.

---

## Section 2.5.C — Production Benchmark Requirements

### F13. Item 6 mislabels the *initial* $MinScore$ value as the *floor* (Medium)

> **Original:** *"Kiểm chứng Hằng số Thang điểm ($S_{boost}$ vs $MinScore$): Kiểm tra thực tế xem trần $S_{boost} \le 30.0$ và **sàn** $MinScore = 60.0$ có hoạt động ổn định trên phân phối dữ liệu thật."*

**Why problematic:** "Sàn" means *floor* (minimum bound). But §3.2 explicitly defines $60.0$ as the **starting** threshold (`order_attempt 0`) that then *decays downward*, and states the actual floor separately: *"Ngưỡng điểm không được giảm xuống dưới sàn: $MinScore \ge 30.0$."* Item 6 here calls $60.0$ the floor, directly contradicting §3.2's own terminology for the same variable. A QA engineer reading only this benchmark checklist would test the wrong number as the safety-critical lower bound.

**Revised:**
> *"Kiểm chứng Hằng số Thang điểm ($S_{boost}$ vs $MinScore$): Kiểm tra thực tế xem trần $S_{boost} \le 30.0$, ngưỡng khởi điểm $MinScore_{start} = 60.0$, và **sàn tối thiểu** $MinScore_{floor} = 30.0$ (theo Mục 3.2) có hoạt động ổn định trên phân phối dữ liệu thật."*

**Why better:** Uses the same start-vs-floor terminology as §3.2, so the benchmark checklist tests the value that's actually safety-critical (the floor) under its correct name, rather than conflating it with the starting threshold.

No other change needed in §2.5.C — items 1–5 (candidate pool sizing, EMA/hysteresis tuning range, adaptive batch window, TTL A/B test, OSRM p99 budget) are clear, appropriately deferred to empirical validation, and consistent with the constants used elsewhere in the document.

---

## Section 3.1 — Tie-Breaking Rules

### F14. Priority 3 uses a metric explicitly inactive in v1.0 (Medium)

> **Original:** *"Ưu tiên 3: Tài xế có **số dư ví tài khoản lớn hơn**."*

**Why problematic:** This is wallet balance — the same underlying signal as $L_{cash}$ in §1.5, which is explicitly tagged *"Phase 2 — Chưa active trong v1.0."* A live, operative tie-break rule cannot depend on a field the document itself says isn't wired up yet; if two drivers tie through Priority 1 and 2, the system has no defined v1.0 behavior.

**Revised:**
> *"Ưu tiên 3: Tài xế có **Đánh Giá Sao ($R_{star}$) cao hơn**."*

**Why better:** $R_{star}$ is an active v1.0 metric already central to the scoring model (§2.1), making it a natural, immediately-implementable final tie-break, and removes the dependency on a Phase-2 field. (When $L_{cash}$ activates in Phase 2, it can be reinstated as a fourth tie-break level without disturbing this fix.)

No other change needed in §3.1 — Priority 1 (shorter ETA) and Priority 2 (longer idle time / FIFO) are both active v1.0 metrics and correctly ordered (route efficiency before fairness).

---

## Section 3.2 — MinScore Decay & Terminal State

### F15. Decay sequence is illustrated only for attempts 0–2; no closed form with floor clamp is given (High)

> **Original:** *"Thuật toán tự động giảm 20% ngưỡng điểm tối thiểu qua mỗi lượt thử (`order_attempt 0: 60.0` → `order_attempt 1: 48.0` → `order_attempt 2: 38.4`)."* ... *"Ngưỡng điểm không được giảm xuống dưới sàn: $MinScore \ge 30.0$."* ... *"Giới hạn số lần thử tối đa: $\text{MaxOrderAttempt} = 5$."*

**Why problematic:** The raw exponential ($60.0 \times 0.8^n$) crosses below the stated floor *before* the max-attempt limit is reached: attempt 3 gives $30.72$ (still just above the floor), but attempt 4 gives $24.576$ — already below $30.0$. The document gives the multiplicative rule, the floor, and the max-attempt count as three separate facts, but never states how they combine for attempts 3–5. Does the threshold clamp at exactly $30.0$ starting at attempt 4, or is something else intended? As written, an implementer has to guess.

**Revised:**
> $$MinScore(\text{order\_attempt}) = \max\left(30.0,\ 60.0 \times 0.8^{\text{order\_attempt}}\right)$$
> *"Ví dụ: `attempt 0: 60.0` → `1: 48.0` → `2: 38.4` → `3: 30.72` → `4: 30.0` (clamped) → `5: 30.0` (clamped). Nếu tại `order_attempt = 5` vẫn không tìm được tài xế đạt $MinScore \ge 30.0$, hệ thống chuyển Trạng thái Kết thúc (Terminal State) như mô tả."*

**Why better:** A single closed-form expression replaces three separately-stated facts (decay rate, floor, max attempts) with one unambiguous rule that already produces the exact numbers given in the original three-point example, while also correctly resolving the previously-undefined attempts 3–5.

No other change needed in §3.2 or §3.3 — the terminal-state customer-facing message and the redispatch protocol (`excluded_driver_ids`, `order_attempt++`, `booking.retry_requested` after 500ms) are consistent with §2.5.B and require no changes.

---

## Closing Note

Fifteen findings were identified, three of them (F2, F6, F7) load-bearing enough that a literal implementation of the current text would produce materially wrong rankings or a broken score invariant rather than just a stylistic inconsistency. All proposed revisions preserve the document's existing numeric examples and design philosophy (reciprocal-decay core score, saturating additive boosts, ratio-based regime routing) — none of them change the intended behavior of the system, only make the specification precise enough to implement as written.

If useful, I can produce a fully revised `algo.md` with all fifteen fixes applied inline, in the same structure as the original.
