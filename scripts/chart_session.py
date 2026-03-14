#!/usr/bin/env python3
"""Chart a single market session: trades, inventory imbalance, pricing, and drift."""
import json
import sys
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
from datetime import datetime

def load_ticks(path):
    ticks = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            ticks.append(json.loads(line))
    return ticks

def parse_ts(ts_str):
    # Handle timezone offset format
    for fmt in ["%Y-%m-%dT%H:%M:%S%z", "%Y-%m-%dT%H:%M:%S.%f%z"]:
        try:
            return datetime.strptime(ts_str, fmt)
        except ValueError:
            continue
    return datetime.fromisoformat(ts_str)

def chart(path, out_path=None):
    ticks = load_ticks(path)
    if not ticks:
        print("No ticks found")
        return

    ts = [parse_ts(t["ts"]) for t in ticks]

    # Pricing
    p_raw = [t["p_raw"] for t in ticks]
    p_lo = [t["p_lo"] for t in ticks]
    p_hi = [t["p_hi"] for t in ticks]
    up_ask = [t["up_ask"] for t in ticks]
    down_ask = [t["down_ask"] for t in ticks]

    # Inventory
    up_qty = [t["up_qty"] for t in ticks]
    down_qty = [t["down_qty"] for t in ticks]
    imbalance = [t["up_qty"] - t["down_qty"] for t in ticks]
    floor = [t["guaranteed_floor"] for t in ticks]

    # Drift (new fields — may not exist in old logs)
    has_drift = "drift_delta_z" in ticks[0] and any(t.get("drift_delta_z", 0) != 0 for t in ticks)
    drift_dz = [t.get("drift_delta_z", 0) for t in ticks]
    drift_ps = [t.get("drift_per_sec", 0) for t in ticks]

    # Detect trades by inventory changes
    trade_ts, trade_side, trade_shares = [], [], []
    for i in range(1, len(ticks)):
        du = ticks[i]["up_qty"] - ticks[i-1]["up_qty"]
        dd = ticks[i]["down_qty"] - ticks[i-1]["down_qty"]
        if du > 0:
            trade_ts.append(ts[i])
            trade_side.append("UP")
            trade_shares.append(du)
        if dd > 0:
            trade_ts.append(ts[i])
            trade_side.append("DOWN")
            trade_shares.append(dd)

    n_panels = 5 if has_drift else 4
    ratios = [3, 2, 2, 1.5, 1] if has_drift else [3, 2, 2, 1]
    fig, axes = plt.subplots(n_panels, 1, figsize=(14, 14 if has_drift else 12),
                              sharex=True, gridspec_kw={"height_ratios": ratios})

    slug = path.split("prices_")[-1].replace(".json", "")
    fig.suptitle(f"Market: {slug}", fontsize=14, fontweight="bold")

    # --- Panel 1: Fair probability vs market prices ---
    ax1 = axes[0]
    ax1.fill_between(ts, p_lo, p_hi, alpha=0.2, color="blue", label="p_lo / p_hi")
    ax1.plot(ts, p_raw, color="blue", linewidth=1.5, label="p_raw (model)")
    ax1.plot(ts, up_ask, color="green", linewidth=1, alpha=0.7, label="up_ask (market)")
    ax1.plot(ts, [1 - d for d in down_ask], color="red", linewidth=1, alpha=0.7, label="1 - down_ask")

    # Mark trades
    for t_ts, t_side, t_sh in zip(trade_ts, trade_side, trade_shares):
        color = "green" if t_side == "UP" else "red"
        marker = "^" if t_side == "UP" else "v"
        ax1.axvline(t_ts, color=color, alpha=0.3, linewidth=0.8)
        ax1.scatter([t_ts], [0.5], color=color, marker=marker, s=60, zorder=5)

    ax1.set_ylabel("Probability")
    ax1.set_ylim(0, 1)
    ax1.legend(loc="upper right", fontsize=8)
    ax1.set_title("Fair Probability vs Market")
    ax1.grid(True, alpha=0.3)

    # --- Panel 2: Inventory ---
    ax2 = axes[1]
    ax2.plot(ts, up_qty, color="green", linewidth=1.5, label="UP shares")
    ax2.plot(ts, down_qty, color="red", linewidth=1.5, label="DOWN shares")
    ax2.fill_between(ts, 0, [min(u, d) for u, d in zip(up_qty, down_qty)],
                     alpha=0.2, color="gray", label="balanced (hedged)")

    for t_ts, t_side, t_sh in zip(trade_ts, trade_side, trade_shares):
        color = "green" if t_side == "UP" else "red"
        y = max(up_qty) * 0.9 if t_side == "UP" else max(down_qty) * 0.9 if down_qty else 0
        ax2.annotate(f"+{t_sh:.0f}{t_side[0]}", xy=(t_ts, 0), fontsize=7,
                    color=color, rotation=90, va="bottom")

    ax2.set_ylabel("Shares")
    ax2.legend(loc="upper left", fontsize=8)
    ax2.set_title("Inventory (UP vs DOWN shares)")
    ax2.grid(True, alpha=0.3)

    # --- Panel 3: Imbalance ---
    ax3 = axes[2]
    ax3.fill_between(ts, 0, imbalance, where=[v >= 0 for v in imbalance],
                     color="green", alpha=0.3, label="Long UP")
    ax3.fill_between(ts, 0, imbalance, where=[v < 0 for v in imbalance],
                     color="red", alpha=0.3, label="Long DOWN")
    ax3.plot(ts, imbalance, color="black", linewidth=1)
    ax3.axhline(0, color="black", linewidth=0.5, linestyle="--")
    ax3.set_ylabel("Imbalance (UP - DOWN)")
    ax3.legend(loc="upper left", fontsize=8)
    ax3.set_title("Inventory Imbalance")
    ax3.grid(True, alpha=0.3)

    # --- Panel 4: Drift delta_z (NEW) ---
    if has_drift:
        ax_drift = axes[3]
        ax_drift.fill_between(ts, 0, drift_dz, where=[v >= 0 for v in drift_dz],
                             color="green", alpha=0.4, label="Bullish drift")
        ax_drift.fill_between(ts, 0, drift_dz, where=[v < 0 for v in drift_dz],
                             color="red", alpha=0.4, label="Bearish drift")
        ax_drift.plot(ts, drift_dz, color="black", linewidth=0.8)
        ax_drift.axhline(0, color="black", linewidth=0.5, linestyle="--")
        ax_drift.axhline(0.5, color="gray", linewidth=0.5, linestyle=":", alpha=0.5)
        ax_drift.axhline(-0.5, color="gray", linewidth=0.5, linestyle=":", alpha=0.5)

        # Mark trades on drift panel too
        for t_ts, t_side, t_sh in zip(trade_ts, trade_side, trade_shares):
            color = "green" if t_side == "UP" else "red"
            ax_drift.axvline(t_ts, color=color, alpha=0.3, linewidth=0.8)

        ax_drift.set_ylabel("drift_delta_z")
        ax_drift.legend(loc="upper left", fontsize=8)
        ax_drift.set_title("EWMA Drift (z-score shift, cap ±0.5)")
        ax_drift.grid(True, alpha=0.3)

    # --- Panel 5 (or 4): Guaranteed Floor ---
    ax_floor = axes[-1]
    ax_floor.plot(ts, floor, color="purple", linewidth=1.5)
    ax_floor.fill_between(ts, 0, floor, where=[f >= 0 for f in floor],
                     color="green", alpha=0.2)
    ax_floor.fill_between(ts, 0, floor, where=[f < 0 for f in floor],
                     color="red", alpha=0.2)
    ax_floor.axhline(0, color="black", linewidth=0.5, linestyle="--")
    ax_floor.set_ylabel("Floor ($)")
    ax_floor.set_title("Guaranteed Floor (G)")
    ax_floor.grid(True, alpha=0.3)

    ax_floor.xaxis.set_major_formatter(mdates.DateFormatter("%H:%M:%S"))
    plt.xticks(rotation=45)

    plt.tight_layout()

    if out_path is None:
        out_path = path.replace("prices_", "chart_").replace(".json", ".png")
    plt.savefig(out_path, dpi=150, bbox_inches="tight")
    print(f"Saved: {out_path}")
    plt.close()

if __name__ == "__main__":
    log_dir = "/Users/alimohammad/GolandProjects/Polybot/logs"
    # Chart the two most recent drift-active sessions
    files = [
        f"{log_dir}/prices_btc-updown-5m-1773438600.json",
        f"{log_dir}/prices_btc-updown-5m-1773438900.json",
    ]
    for f in files:
        chart(f)
