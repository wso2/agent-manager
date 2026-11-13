"""Manifest parser and matrix expansion.

The manifest is the single source of truth for what cells exist. This module
parses matrix.yaml (or a fixture) into typed records and expands them into the
flat list of Cell objects the harness iterates over.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

import yaml


@dataclass
class ProviderEntry:
    name: str
    versions: list[str]
    contract_schema: str
    # Maps provider-version → the amp-instrumentation release that pins it.
    # Used by the heavy tier to install `amp-instrumentation==<version>`.
    # Optional and unset for providers without such a release (e.g., the
    # `manual` provider) — those leave `Cell.instrumentation_version` as None.
    instrumentation_versions: dict[str, str] = field(default_factory=dict)


@dataclass
class FrameworkEntry:
    name: str
    package: str
    versions: list[str]
    sample_path: str
    span_kinds: list[str]
    extras: list[str] = field(default_factory=list)
    provider_restriction: Optional[str] = None


@dataclass
class DefaultCell:
    provider: str
    provider_version: str
    framework: str
    framework_version: str
    python: str


@dataclass
class HeavyTier:
    per_traceloop_version: int
    per_framework: int


@dataclass
class KnownBroken:
    cell_match: dict[str, str]
    reason: str
    until: str


@dataclass
class Manifest:
    schema_version: int
    providers: dict[str, ProviderEntry]
    frameworks: list[FrameworkEntry]
    python_versions: list[str]
    default_cell: DefaultCell
    heavy_tier: HeavyTier
    known_broken: list[KnownBroken] = field(default_factory=list)


@dataclass
class Cell:
    provider_name: str
    provider_version: str
    framework_name: str
    framework_version: str
    framework_package: str
    sample_path: str
    span_kinds: list[str]
    python: str
    contract_schema: str
    extras: list[str] = field(default_factory=list)
    # amp-instrumentation release for the heavy tier, resolved from the
    # provider's instrumentation_versions map. None when the provider has no
    # such release (manual provider) or when the mapping is absent.
    instrumentation_version: Optional[str] = None

    @property
    def id(self) -> str:
        return (
            f"{self.provider_name}-{self.provider_version}-"
            f"{self.framework_name}-{self.framework_version}-py{self.python}"
        )


def load_manifest(path: Path) -> Manifest:
    raw = yaml.safe_load(Path(path).read_text())

    providers = {
        name: ProviderEntry(
            name=name,
            versions=p["versions"],
            contract_schema=p["contractSchema"],
            instrumentation_versions=p.get("instrumentationVersions", {}) or {},
        )
        for name, p in raw["providers"].items()
    }
    frameworks = [
        FrameworkEntry(
            name=f["name"],
            package=f["package"],
            versions=f["versions"],
            sample_path=f["samplePath"],
            span_kinds=f["spanKinds"],
            extras=f.get("extras", []),
            provider_restriction=f.get("provider"),
        )
        for f in raw["frameworks"]
    ]
    default_cell = DefaultCell(
        provider=raw["defaultCell"]["provider"],
        provider_version=raw["defaultCell"]["providerVersion"],
        framework=raw["defaultCell"]["framework"],
        framework_version=raw["defaultCell"]["frameworkVersion"],
        python=raw["defaultCell"]["python"],
    )
    heavy_tier = HeavyTier(
        per_traceloop_version=raw["heavyTier"]["perTraceloopVersion"],
        per_framework=raw["heavyTier"]["perFramework"],
    )
    known_broken = [
        KnownBroken(cell_match=kb["cell"], reason=kb["reason"], until=kb["until"])
        for kb in raw.get("known-broken", [])
    ]

    return Manifest(
        schema_version=raw["schemaVersion"],
        providers=providers,
        frameworks=frameworks,
        python_versions=raw["python"]["versions"],
        default_cell=default_cell,
        heavy_tier=heavy_tier,
        known_broken=known_broken,
    )


def expand_matrix(manifest: Manifest) -> list[Cell]:
    cells: list[Cell] = []
    for fw in manifest.frameworks:
        provider_names = (
            [fw.provider_restriction]
            if fw.provider_restriction
            else list(manifest.providers.keys())
        )
        for pname in provider_names:
            if pname not in manifest.providers:
                # An unknown provider name on a framework's `provider:` is
                # almost certainly a typo (e.g., `tracelop`) — silently
                # skipping would make the framework produce zero cells with
                # no error. Fail loud at expansion time.
                raise ValueError(
                    f"framework '{fw.name}' is restricted to provider "
                    f"'{pname}' which is not declared under matrix.yaml "
                    f"providers: (got {sorted(manifest.providers.keys())})"
                )
            provider = manifest.providers[pname]
            for pver in provider.versions:
                for fver in fw.versions:
                    for py in manifest.python_versions:
                        cells.append(
                            Cell(
                                provider_name=pname,
                                provider_version=pver,
                                framework_name=fw.name,
                                framework_version=fver,
                                framework_package=fw.package,
                                sample_path=fw.sample_path,
                                # Copy the framework's lists so cells don't
                                # share a mutable instance (a mutation on one
                                # cell would otherwise leak into all of them).
                                span_kinds=list(fw.span_kinds),
                                python=py,
                                contract_schema=provider.contract_schema,
                                extras=list(fw.extras),
                                instrumentation_version=(
                                    provider.instrumentation_versions.get(pver)
                                ),
                            )
                        )
    return cells
