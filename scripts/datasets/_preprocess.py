"""
Shared driver for per-dataset preprocessors.

Each dataset preprocessor supplies 2 things:
    iter_raw_records(input_path) -> Iterator[dict]
        Parses the raw format persisted on HuggingFace Hub.

    process_record(record, tokenizer) -> tuple[dict | None, str | None]
        Per-record decision maker. Return (kept_dict, None) to keep a record, else
        (None, "some reason") to drop it with a labeled reason.
        The driver counts and reports whatever reasons coming back.

...then calls: entrypoint(iter_raw_records, process_record, description=__doc__)
"""

import argparse
import json
import logging
import sys
from collections import Counter
from pathlib import Path
from typing import Callable, Iterator

from _common import assert_in_venv, default_tokenizer_path, repo_root

assert_in_venv()  # Use third-party library

from tokenizers import Tokenizer

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger(__name__)

ProcessFn = Callable[[dict, Tokenizer], "tuple[dict | None, str | None]"]
IterFn = Callable[[Path], Iterator[dict]]


def load_default_tokenizer() -> Tokenizer:
    path = default_tokenizer_path()
    if not path.exists():
        log.error(f"Tokenizer not found: {path}. Run a transformer fetch script first.")
        sys.exit(1)

    log.info(f"Using tokenizer {path}")
    tok = Tokenizer.from_file(str(path))
    tok.no_padding()
    tok.no_truncation()
    return tok


def count_tokens(tok: Tokenizer, text: str) -> int:
    return len(tok.encode(text).ids)


def run(input_path: Path, output_path: Path, iter_raw_records: IterFn, process_record: ProcessFn):
    tok = load_default_tokenizer()
    output_path.parent.mkdir(parents=True, exist_ok=True)

    total = kept = 0
    drop_reasons: Counter[str] = Counter()
    flagged: Counter[str] = Counter()

    with output_path.open("w", encoding="utf-8") as fout:
        for record in iter_raw_records(input_path):
            total += 1

            result, tag = process_record(record, tok)
            if result is None:
                drop_reasons[tag or "unspecified"] += 1
                continue
            if tag:
                flagged[tag] += 1

            fout.write(json.dumps(result, ensure_ascii=True) + '\n')
            kept += 1

    log.info("-" * 50)
    log.info("Total records : %d", total)
    log.info("Kept          : %d  (%.1f%%)", kept, 100 * kept / max(total, 1))
    for reason, count in drop_reasons.most_common():
        log.info("Dropped (%-12s): %d", reason, count)
    for tag, count in flagged.most_common():
        log.info("Flagged (%-12s): %d  (kept, not excluded)", tag, count)
    log.info("Output        : %s", output_path.resolve())


def parse_args(description: str | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=description,
        formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("input", type=Path, help="Path to the raw dataset file")
    return parser.parse_args()


def entrypoint(iter_raw_records: IterFn, process_record: ProcessFn,
               description: str | None = None) -> int:
    args = parse_args(description)

    input_path = args.input.resolve()
    if not input_path.exists():
        log.error(f"Raw input dataset not found: {args.input}")
        return 1
    if not input_path.is_file():
        log.error(f"Input path must be a file, not a directory: {args.input}")
        return 1

    dataset_name = input_path.stem
    output_path = repo_root() / "data" / f"{dataset_name}.jsonl"
    run(input_path, output_path, iter_raw_records, process_record)
    return 0
