#!/usr/bin/env python3
"""
Pricing session analyzer for Polybot binary options.

Loads pricing log JSONs, computes calibration metrics (model p_raw vs market mid),
analyzes edge signals, computes paper P&L, and saves diagnostic charts.
"""

import json
import sys
import os
from pathlib import Path
from datetime import datetime

import numpy as np
import pandas as pd
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.dates as mdates


LOG_DIR = Path(__file__).resolve().parent.parent / "logs"

FILES = [
    "prices_btc-updown-5m-1773422700.json",
    "prices_btc-updown-5m-1773423000.json",
]


def load_log(path: Path) -> pd.DataFrame:
    records = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            records.append(json.loads(line))
    df = pd.DataFrame(records)
    df["ts"] = pd.to_datetime(df["ts"])
    # Derive market_id from filename
    df["market_id"] = path.stem.replace("prices_", "")
    return df


def market_mid_up(row):
    """Market-implied probability of UP from mid of up_bid/up_ask."""
    bid, ask = row["up_bid"], row["up_ask"]
    if bid == 0 and ask == 0:
        return 0.0
    return (bid + ask) / 2.0


def compute_metrics(df: pd.DataFrame, label: str):
    """Compute and print calibration metrics for a single market."""
    df = df.copy()
    df["market_mid"] = df.apply(market_mid_up, axis=1)
    df["error"] = df["p_raw"] - df["market_mid"]  # model - market
    df["abs_error"] = df["error"].abs()
    df["remaining_sec"] = df["remaining_ms"] / 1000.0

    n = len(df)
    mae = df["abs_error"].mean()
    rmse = np.sqrt((df["error"] ** 2).mean())
    bias = df["error"].mean()
    max_err = df["abs_error"].max()
    within_1pct = (df["abs_error"] <= 0.01).sum() / n * 100
    within_5pct = (df["abs_error"] <= 0.05).sum() / n * 100
    within_10pct = (df["abs_error"] <= 0.10).sum() / n * 100

    print(f"\n{'='*60}")
    print(f"  Market: {label}")
    print(f"  Records: {n}  |  Time range: {df['ts'].min()} -> {df['ts'].max()}")
    print(f"  Price to beat: {df['price_to_beat'].iloc[0]:.2f}")
    print(f"  Ref price (first): {df['ref_price'].iloc[0]:.2f}  (last): {df['ref_price'].iloc[-1]:.2f}")
    print(f"{'='*60}")

    print(f"\n  --- Error Statistics (model p_raw - market mid) ---")
    print(f"  MAE:       {mae:.6f}  ({mae*100:.2f} pp)")
    print(f"  RMSE:      {rmse:.6f}  ({rmse*100:.2f} pp)")
    print(f"  Bias:      {bias:+.6f}  ({bias*100:+.2f} pp)  {'OVER-prices UP' if bias > 0 else 'UNDER-prices UP'}")
    print(f"  Max |err|: {max_err:.6f}  ({max_err*100:.2f} pp)")
    print(f"  Within 1pp:  {within_1pct:.1f}%")
    print(f"  Within 5pp:  {within_5pct:.1f}%")
    print(f"  Within 10pp: {within_10pct:.1f}%")

    # Time buckets
    print(f"\n  --- Error by remaining time bucket ---")
    bins = [0, 30, 60, 120, 180, 300, 999]
    labels_b = ["0-30s", "30-60s", "60-120s", "120-180s", "180-300s", "300s+"]
    df["time_bucket"] = pd.cut(df["remaining_sec"], bins=bins, labels=labels_b, right=True)
    for b in labels_b:
        sub = df[df["time_bucket"] == b]
        if len(sub) == 0:
            continue
        b_mae = sub["abs_error"].mean()
        b_bias = sub["error"].mean()
        print(f"    {b:>10s}: n={len(sub):4d}  MAE={b_mae:.4f} ({b_mae*100:.1f}pp)  bias={b_bias:+.4f} ({b_bias*100:+.1f}pp)")

    # Action distribution
    print(f"\n  --- Action distribution ---")
    for action, count in df["action"].value_counts().items():
        print(f"    {action}: {count}")

    return df


def compute_pnl(df: pd.DataFrame, label: str):
    """Compute paper P&L from final position snapshot."""
    last = df.iloc[-1]
    up_qty = last["up_qty"]
    down_qty = last["down_qty"]
    up_cost = last["up_cost"]
    down_cost = last["down_cost"]
    total_cost = up_cost + down_cost

    # For market 1773422700: price_to_beat = 71896.62, final ref ~71668
    # BTC did NOT go up -> DOWN wins -> payout = 1 per DOWN share
    # outcome: UP = 0, DOWN = 1 (since ref_price < price_to_beat at expiry)
    ref_final = last["ref_price"]
    ptb = last["price_to_beat"]
    up_won = ref_final >= ptb

    payout_per_up = 1.0 if up_won else 0.0
    payout_per_down = 0.0 if up_won else 1.0

    gross_payout = up_qty * payout_per_up + down_qty * payout_per_down
    # Fee = 1% of winnings (profit portion only)
    winnings = max(0, gross_payout - total_cost)
    fee = 0.01 * winnings
    net_pnl = gross_payout - total_cost - fee

    # Also compute guaranteed floor from the log
    g_floor = last["guaranteed_floor"]

    print(f"\n  --- Paper P&L: {label} ---")
    print(f"  Outcome: {'UP won' if up_won else 'DOWN won'}  (ref={ref_final:.2f} vs ptb={ptb:.2f})")
    print(f"  Position: {up_qty:.1f} UP shares @ avg {up_cost/up_qty:.4f} each" if up_qty > 0 else "  Position: 0 UP shares")
    print(f"            {down_qty:.1f} DOWN shares @ avg {down_cost/down_qty:.4f} each" if down_qty > 0 else "            0 DOWN shares")
    print(f"  Total cost:    ${total_cost:.2f}")
    print(f"  Gross payout:  ${gross_payout:.2f}")
    print(f"  Fee (1%):      ${fee:.2f}")
    print(f"  Net P&L:       ${net_pnl:+.2f}")
    print(f"  Log guar_floor: ${g_floor:.2f}")
    print(f"  ROI:           {net_pnl/total_cost*100:+.1f}%" if total_cost > 0 else "  ROI: N/A")

    return {
        "label": label,
        "up_won": up_won,
        "up_qty": up_qty,
        "down_qty": down_qty,
        "total_cost": total_cost,
        "gross_payout": gross_payout,
        "fee": fee,
        "net_pnl": net_pnl,
    }


def plot_calibration(df: pd.DataFrame, label: str, out_dir: Path):
    """Chart 1: p_raw vs market_mid over time."""
    fig, axes = plt.subplots(3, 1, figsize=(14, 12), sharex=True)
    fig.suptitle(f"Calibration Analysis: {label}", fontsize=14, fontweight="bold")

    t = df["ts"]
    remaining = df["remaining_sec"]

    # Panel 1: probabilities
    ax = axes[0]
    ax.plot(t, df["p_raw"], label="Model p_raw", linewidth=1.5, color="blue")
    ax.plot(t, df["market_mid"], label="Market mid (UP)", linewidth=1.5, color="orange", alpha=0.8)
    ax.fill_between(t, df["up_bid"], df["up_ask"], alpha=0.2, color="orange", label="Market bid-ask")
    ax.set_ylabel("P(UP)")
    ax.legend(loc="upper right", fontsize=9)
    ax.set_title("Model vs Market Probability")
    ax.grid(True, alpha=0.3)

    # Panel 2: error
    ax = axes[1]
    ax.plot(t, df["error"] * 100, color="red", linewidth=1, label="Error (model-market) pp")
    ax.axhline(0, color="black", linewidth=0.5)
    ax.fill_between(t, df["error"] * 100, 0, alpha=0.3, color="red")
    ax.set_ylabel("Error (pp)")
    ax.legend(loc="upper right", fontsize=9)
    ax.set_title("Pricing Error Over Time")
    ax.grid(True, alpha=0.3)

    # Panel 3: z-score
    ax = axes[2]
    ax.plot(t, df["z"], color="green", linewidth=1.5)
    ax.axhline(0, color="black", linewidth=0.5)
    ax.axhline(-3, color="red", linewidth=0.5, linestyle="--", label="z clamp [-3, +3]")
    ax.axhline(3, color="red", linewidth=0.5, linestyle="--")
    ax.set_ylabel("z-score")
    ax.set_xlabel("Time")
    ax.legend(loc="upper right", fontsize=9)
    ax.set_title("Z-Score Evolution")
    ax.grid(True, alpha=0.3)

    for ax in axes:
        ax.xaxis.set_major_formatter(mdates.DateFormatter("%H:%M:%S"))

    plt.tight_layout()
    fname = out_dir / f"calibration_{label}.png"
    fig.savefig(fname, dpi=150)
    plt.close(fig)
    print(f"  Saved: {fname}")


def plot_edge_analysis(df: pd.DataFrame, label: str, out_dir: Path):
    """Chart 2: edge signals and trade actions."""
    fig, axes = plt.subplots(2, 1, figsize=(14, 8), sharex=True)
    fig.suptitle(f"Edge & Trade Analysis: {label}", fontsize=14, fontweight="bold")

    t = df["ts"]

    # Panel 1: directional edges
    ax = axes[0]
    ax.plot(t, df["dir_edge_up"] * 100, label="Edge UP", color="green", linewidth=1)
    ax.plot(t, df["dir_edge_down"] * 100, label="Edge DOWN", color="red", linewidth=1)
    ax.axhline(0, color="black", linewidth=0.5)
    ax.set_ylabel("Edge (pp)")
    ax.legend(loc="upper right", fontsize=9)
    ax.set_title("Directional Edge Over Time")
    ax.grid(True, alpha=0.3)

    # Panel 2: position build-up
    ax = axes[1]
    ax.plot(t, df["up_qty"], label="UP qty", color="blue", linewidth=1.5)
    ax.plot(t, df["down_qty"], label="DOWN qty", color="orange", linewidth=1.5)
    ax2 = ax.twinx()
    ax2.plot(t, df["guaranteed_floor"], label="Guar. floor", color="red", linewidth=1, linestyle="--")
    ax2.set_ylabel("Guaranteed Floor ($)", color="red")
    ax.set_ylabel("Share Quantity")
    ax.legend(loc="upper left", fontsize=9)
    ax2.legend(loc="upper right", fontsize=9)
    ax.set_title("Position & Risk Over Time")
    ax.grid(True, alpha=0.3)

    for ax in axes:
        ax.xaxis.set_major_formatter(mdates.DateFormatter("%H:%M:%S"))

    plt.tight_layout()
    fname = out_dir / f"edge_analysis_{label}.png"
    fig.savefig(fname, dpi=150)
    plt.close(fig)
    print(f"  Saved: {fname}")


def plot_sigma_tau(df: pd.DataFrame, label: str, out_dir: Path):
    """Chart 3: sigma_tau decay and its effect on pricing precision."""
    fig, axes = plt.subplots(2, 1, figsize=(14, 8), sharex=True)
    fig.suptitle(f"Volatility & Time Decay: {label}", fontsize=14, fontweight="bold")

    t = df["ts"]

    # Panel 1: sigma_tau
    ax = axes[0]
    ax.plot(t, df["sigma_tau"], color="purple", linewidth=1.5)
    ax.set_ylabel("sigma_tau")
    ax.set_title("sigma_tau (Total Vol to Expiry) Decay")
    ax.grid(True, alpha=0.3)

    # Panel 2: remaining time and abs error
    ax = axes[1]
    ax.scatter(df["remaining_sec"], df["abs_error"] * 100, s=3, alpha=0.5, color="red")
    ax.set_xlabel("Remaining Time (sec)")
    ax.set_ylabel("|Error| (pp)")
    ax.set_title("Absolute Error vs Remaining Time")
    ax.grid(True, alpha=0.3)
    ax.invert_xaxis()

    plt.tight_layout()
    fname = out_dir / f"vol_decay_{label}.png"
    fig.savefig(fname, dpi=150)
    plt.close(fig)
    print(f"  Saved: {fname}")


def analyze_sigma_jump(df: pd.DataFrame, label: str):
    """Detect large sigma_tau jumps (potential vol floor activation)."""
    df = df.copy()
    df["sigma_tau_pct_change"] = df["sigma_tau"].pct_change()
    big_jumps = df[df["sigma_tau_pct_change"].abs() > 0.5]
    if len(big_jumps) > 0:
        print(f"\n  --- Large sigma_tau jumps in {label} ---")
        for _, row in big_jumps.iterrows():
            print(f"    {row['ts']}: sigma_tau={row['sigma_tau']:.6f} change={row['sigma_tau_pct_change']:+.1%}")
    else:
        print(f"\n  No large sigma_tau jumps in {label}")

    # Check vol floor: sigma_per_sec = sigma_tau / sqrt(remaining_sec)
    df["remaining_sec"] = df["remaining_ms"] / 1000.0
    df["sigma_per_sec"] = df["sigma_tau"] / np.sqrt(df["remaining_sec"])
    floor_count = (df["sigma_per_sec"].round(6) <= 0.000121).sum()
    total = len(df)
    print(f"  Vol floor active (sigma/sec ~ 0.00012): {floor_count}/{total} ticks ({floor_count/total*100:.1f}%)")
    print(f"  sigma_per_sec range: [{df['sigma_per_sec'].min():.6f}, {df['sigma_per_sec'].max():.6f}]")


def main():
    print("=" * 60)
    print("  POLYBOT PRICING SESSION ANALYSIS")
    print("=" * 60)

    all_pnl = []

    for fname in FILES:
        path = LOG_DIR / fname
        if not path.exists():
            print(f"WARNING: {path} not found, skipping")
            continue

        label = path.stem.replace("prices_", "")
        df = load_log(path)
        df = compute_metrics(df, label)

        # Sigma analysis
        analyze_sigma_jump(df, label)

        # P&L
        pnl = compute_pnl(df, label)
        all_pnl.append(pnl)

        # Charts
        print(f"\n  Generating charts for {label}...")
        plot_calibration(df, label, LOG_DIR)
        plot_edge_analysis(df, label, LOG_DIR)
        plot_sigma_tau(df, label, LOG_DIR)

    # Combined P&L summary
    if all_pnl:
        print(f"\n{'='*60}")
        print(f"  COMBINED SESSION P&L")
        print(f"{'='*60}")
        total_cost = sum(p["total_cost"] for p in all_pnl)
        total_payout = sum(p["gross_payout"] for p in all_pnl)
        total_fee = sum(p["fee"] for p in all_pnl)
        total_pnl = sum(p["net_pnl"] for p in all_pnl)
        for p in all_pnl:
            print(f"  {p['label']}: ${p['net_pnl']:+.2f} (cost=${p['total_cost']:.2f}, payout=${p['gross_payout']:.2f})")
        print(f"  ---")
        print(f"  Total cost:   ${total_cost:.2f}")
        print(f"  Total payout: ${total_payout:.2f}")
        print(f"  Total fees:   ${total_fee:.2f}")
        print(f"  Net P&L:      ${total_pnl:+.2f}")
        if total_cost > 0:
            print(f"  Session ROI:  {total_pnl/total_cost*100:+.1f}%")

    print(f"\nDone.")


if __name__ == "__main__":
    main()
