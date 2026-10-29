"""
Shared helpers for dataset-processing scripts.
"""

import os
import re
import subprocess
import sys
import tomllib
from dataclasses import dataclass
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


REPO_ID_FIELD = "repo_id"
_REPO_ID_REGEX = re.compile(
    r"^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$"
)  # No whitespace, single forward slash '/'

REVISION_FIELD = "revision"
FILE_PATH_FIELD = "file_path"

LOCAL_NAME_FIELD = "local_name"
_LOCAL_NAME_REGEX = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]*$")  # No whitespace

REQUIRED_MANIFEST_KEYS = {REPO_ID_FIELD, REVISION_FIELD, FILE_PATH_FIELD, LOCAL_NAME_FIELD}


def _commit_hash_lookalike(revision: str) -> bool:
    return len(revision) == 40 and all(c in "0123456789abcdef" for c in revision.lower())


@dataclass(frozen=True)
class Manifest:
    repo_id: str
    revision: str
    file_path: str
    local_name: str

    @property
    def raw_text(self) -> str:
        """Original dataset extension on HuggingFace"""
        return Path(self.file_path).suffix


def load_manifest(info_toml: Path) -> Manifest:
    """Read manifest file from specified path and sanitizes its values"""
    with info_toml.open("rb") as fin:
        raw = tomllib.load(fin)

    missing = REQUIRED_MANIFEST_KEYS - set(raw.keys())
    if missing:
        sys.exit(f"{info_toml} - missing required key(s): {sorted(missing)}.")

    repo_id = raw[REPO_ID_FIELD]
    if not isinstance(repo_id, str) or not _REPO_ID_REGEX.fullmatch(repo_id):
        sys.exit(
            f"{info_toml} - invalid '{REPO_ID_FIELD}': {repo_id!r}.\n"
            "Expected format \"owner/dataset_name\" with no whitespace and a single '/'."
        )

    revision = raw[REVISION_FIELD]
    if not isinstance(revision, str) or not _commit_hash_lookalike(revision):
        sys.exit(
            f"{info_toml} - invalid '{REVISION_FIELD}': {revision!r}.\n"
            "Expected a 40-character commit hash."
        )

    file_path = raw[FILE_PATH_FIELD]
    if not isinstance(file_path, str) or not file_path.strip():
        sys.exit(f"{info_toml} - '{FILE_PATH_FIELD}' must be a non-empty string")

    local_name = raw[LOCAL_NAME_FIELD]
    if not isinstance(local_name, str) or not _LOCAL_NAME_REGEX.fullmatch(local_name):
        sys.exit(
            f"{info_toml} - invalid '{LOCAL_NAME_FIELD}': {local_name!r}.\n"
            "Expected only [A-Za-z0-9_-], no whitespace or dot."
        )

    return Manifest(repo_id=repo_id, revision=revision, file_path=file_path, local_name=local_name)
