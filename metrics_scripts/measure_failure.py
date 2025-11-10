import sys
import argparse
from collections import Counter, defaultdict
from datetime import datetime

#!/usr/bin/env python3
"""
measure_failure.py

Usage:
    - Provide a log file as the first argument, or pipe into stdin.
    - Lines expected: "<ISO8601 timestamp> OK" or "<ISO8601 timestamp> FAIL"
    - Example: 2025-11-10T20:25:41+00:00 FAIL

Prints overall failure percent and optional per-minute breakdown with --per-minute.
"""

def parse_line(line):
        line = line.strip()
        if not line:
                return None, None
        parts = line.split()
        if len(parts) < 2:
                return None, None
        ts_str = parts[0]
        status = parts[1].upper()
        try:
                ts = datetime.fromisoformat(ts_str)
        except Exception:
                return None, None
        return ts, status

def main():
        p = argparse.ArgumentParser(description="Measure failure percent in OK/FAIL logs")
        p.add_argument("file", nargs="?", help="log file (default: stdin)")
        p.add_argument("--per-minute", action="store_true", help="show per-minute failure percent")
        args = p.parse_args()

        f = open(args.file, "r") if args.file else sys.stdin

        counts = Counter()
        per_min = defaultdict(Counter)

        for i, line in enumerate(f, 1):
                ts, status = parse_line(line)
                if ts is None or status not in ("OK", "FAIL"):
                        # ignore malformed lines but report once
                        continue
                counts[status] += 1
                if args.per_minute:
                        bucket = ts.replace(second=0, microsecond=0)
                        per_min[bucket][status] += 1

        if args.file:
                f.close()

        total = counts["OK"] + counts["FAIL"]
        if total == 0:
                print("No OK/FAIL entries found.")
                return

        fail_pct = counts["FAIL"] / total * 100.0
        print(f"total={total} ok={counts['OK']} fail={counts['FAIL']} fail_pct={fail_pct:.2f}%")

        if args.per_minute:
                print("\nPer-minute failure percent:")
                for minute in sorted(per_min):
                        c = per_min[minute]
                        t = c["OK"] + c["FAIL"]
                        pct = (c["FAIL"] / t * 100.0) if t else 0.0
                        # ISO minute label (no seconds)
                        label = minute.isoformat()
                        print(f"{label} total={t} ok={c['OK']} fail={c['FAIL']} fail_pct={pct:.2f}%")

if __name__ == "__main__":
        main()