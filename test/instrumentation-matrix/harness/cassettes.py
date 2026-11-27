"""Resolve the directory shared by cells that replay the same cassette."""
from pathlib import Path


def cassette_directory(root: Path, framework: str, version: str) -> Path:
    framework_dir = root / framework
    version_dir = framework_dir / version
    return version_dir if version_dir.is_dir() else framework_dir
