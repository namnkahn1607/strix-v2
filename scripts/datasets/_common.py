"""
Shared helpers for dataset-processing scripts.
"""

import os
import sys
import subprocess
from pathlib import Path

# Token limit invariant enforce by all-MiniLM-L6-v2.
MAX_BOUND_TOKENS = 256


def assert_in_venv():
    """
    Verify if the calling script is inside a virtual environment.
    MUST be called before any third-party import.
    """
    if sys.prefix == sys.base_prefix:
        sys.exit("This script uses third-party libraries. Run it inside a venv.\n"
                 "Fix: Move cwd to scripts/ and re-run.\n")


def repo_root() -> Path:
    """Resolve repository root from the script's directory"""
    script_dir = Path(__file__).resolve().parent
    result = subprocess.run(
        ["git", "-C", str(script_dir), "rev-parse", "--show-toplevel"],
        capture_output=True, text=True, check=True
    )
    root = result.stdout.strip()
    assert root, "Cannot resolve repository root. Is this script inside a git checkout?"
    return Path(root)


def default_tokenizer_path() -> Path:
    """Default path to tokenizer.json"""
    return repo_root() / "model" / "tokenizer.json"


def default_cache_dir() -> Path:
    """Raw fetched datasets caching directory"""
    base = os.environ.get("XDG_CACHE_HOME") or str(Path.home() / ".cache")
    return Path(base) / "strix" / "datasets"
