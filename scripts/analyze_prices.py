#!/usr/bin/env python3
"""Analyze Polybot price logs: model probabilities vs market prices."""

import argparse
import sys
from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.dates as mdates
import pandas as pd


def load_log(path: Path) -> pd.DataFrame:
    df = pd.read_json(path, lines=True)
    df["ts"] = pd.to_datetime(df["ts"])
    return df


def plot_market(df: pd.DataFrame, slug: str):
    fig, axes = plt.subplots(4, 1, figsize=(16, 14), sharex=True)
    fig.suptitle(slug, fontsize=14, fontweight="bold")

    # --- Panel 1: Model probability vs market prices ---
    ax = axes[0]
    ax.plot(df["ts"], df["p_raw"], label="p_raw (model)", color="blue", alpha=0.8)
    ax.plot(df["ts"], df["p_cal"], label="p_cal (calibrated)", color="dodgerblue", alpha=0.8, linestyle="--")
    ax.fill_between(df["ts"], df["p_lo"], df["p_hi"], alpha=0.15, color="blue", label="model bounds")
    ax.plot(df["ts"], df["up_ask"], label="up_ask (market)", color="red", alpha=0.8)
    ax.plot(df["ts"], df["down_ask"], label="down_ask (market)", color="green", alpha=0.8)
    ax.set_ylabel("Probability / Price")
    ax.set_ylim(-0.05, 1.05)
    ax.legend(loc="upper left", fontsize=8)
    ax.set_title("Model Probability vs Market Prices")
    ax.grid(True, alpha=0.3)

    # --- Panel 2: Directional edges ---
    ax = axes[1]
    ax.plot(df["ts"], df["dir_edge_up"], label="edge_up", color="blue", alpha=0.8)
    ax.plot(df["ts"], df["dir_edge_down"], label="edge_down", color="red", alpha=0.8)
    ax.axhline(y=0.03, color="gray", linestyle=":", alpha=0.5, label="hurdle (0.03)")
    ax.axhline(y=0, color="black", linestyle="-", alpha=0.3)
    ax.set_ylabel("Edge")
    ax.legend(loc="upper left", fontsize=8)
    ax.set_title("Directional Edges")
    ax.grid(True, alpha=0.3)

    # --- Panel 3: Hedge edges + floor ---
    ax = axes[2]
    ax.plot(df["ts"], df["hedge_edge_buy_up"], label="hedge_buy_up", color="blue", alpha=0.8)
    ax.plot(df["ts"], df["hedge_edge_buy_down"], label="hedge_buy_down", color="red", alpha=0.8)
    ax.axhline(y=0, color="black", linestyle="-", alpha=0.3)
    ax2 = ax.twinx()
    ax2.plot(df["ts"], df["guaranteed_floor"], label="floor ($)", color="orange", alpha=0.7, linestyle="--")
    ax2.set_ylabel("Floor ($)", color="orange")
    ax.set_ylabel("Hedge Edge")
    ax.legend(loc="upper left", fontsize=8)
    ax2.legend(loc="upper right", fontsize=8)
    ax.set_title("Hedge Edges & Guaranteed Floor")
    ax.grid(True, alpha=0.3)

    # --- Panel 4: Volatility & z-score ---
    ax = axes[3]
    ax.plot(df["ts"], df["sigma_tau"], label="σ_τ (horizon vol)", color="purple", alpha=0.8)
    ax.set_ylabel("σ_τ", color="purple")
    ax.legend(loc="upper left", fontsize=8)
    ax2 = ax.twinx()
    ax2.plot(df["ts"], df["z"], label="z-score", color="teal", alpha=0.7)
    ax2.set_ylabel("z-score", color="teal")
    ax2.legend(loc="upper right", fontsize=8)
    ax.set_title("Volatility & Z-Score")
    ax.grid(True, alpha=0.3)

    axes[-1].xaxis.set_major_formatter(mdates.DateFormatter("%H:%M:%S"))
    plt.xticks(rotation=45)
    plt.tight_layout()
    return fig


def main():
    parser = argparse.ArgumentParser(description="Analyze Polybot price logs")
    parser.add_argument("files", nargs="*", help="Log files (default: all in logs/)")
    parser.add_argument("--save", action="store_true", help="Save PNGs instead of showing")
    args = parser.parse_args()

    log_dir = Path(__file__).parent.parent / "logs"

    if args.files:
        paths = [Path(f) for f in args.files]
    else:
        paths = sorted(log_dir.glob("prices_*.json"))

    if not paths:
        print("No log files found. Run the bot first to generate logs.", file=sys.stderr)
        sys.exit(1)

    for path in paths:
        print(f"Loading {path.name}...")
        df = load_log(path)
        if df.empty:
            print(f"  Skipped (empty)")
            continue

        print(f"  {len(df)} ticks, {df['ts'].min()} -> {df['ts'].max()}")

        slug = path.stem.removeprefix("prices_")
        fig = plot_market(df, slug)

        if args.save:
            out = path.with_suffix(".png")
            fig.savefig(out, dpi=150)
            print(f"  Saved {out}")
        else:
            plt.show()
        plt.close(fig)


if __name__ == "__main__":
    main()
