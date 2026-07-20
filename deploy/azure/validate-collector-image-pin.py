#!/usr/bin/env python3
"""Validate Azure OTel Collector image pinning parameters.

Bicep 0.41 does not provide stable regex parameter validation. This lightweight
check enforces the equivalent constraints for the checked-in defaults and can
also validate override values by setting OTEL_COLLECTOR_IMAGE_REPOSITORY and
OTEL_COLLECTOR_IMAGE_DIGEST in the environment.
"""

from pathlib import Path
import os
import re
import sys

PARAM_RE = re.compile(r"param\s+(otelCollectorImageRepository|otelCollectorImageDigest)\s+string\s+=\s+'([^']+)'")
DIGEST_RE = re.compile(r"^[0-9a-f]{64}$")
FILES = [
    Path("deploy/azure/main.bicep"),
    Path("deploy/azure/modules/container-app.bicep"),
]


def validate(repository: str, digest: str, source: str) -> list[str]:
    errors: list[str] = []
    if "@" in repository:
        errors.append(f"{source}: otelCollectorImageRepository must not contain @ or an embedded digest")
    if not DIGEST_RE.fullmatch(digest):
        errors.append(f"{source}: otelCollectorImageDigest must match ^[0-9a-f]{{64}}$")
    return errors


def defaults(path: Path) -> tuple[str, str]:
    params = dict(PARAM_RE.findall(path.read_text()))
    return params["otelCollectorImageRepository"], params["otelCollectorImageDigest"]


def main() -> int:
    errors: list[str] = []
    for path in FILES:
        errors.extend(validate(*defaults(path), str(path)))

    env_repository = os.getenv("OTEL_COLLECTOR_IMAGE_REPOSITORY")
    env_digest = os.getenv("OTEL_COLLECTOR_IMAGE_DIGEST")
    if env_repository or env_digest:
        if not env_repository or not env_digest:
            errors.append("environment overrides must set both OTEL_COLLECTOR_IMAGE_REPOSITORY and OTEL_COLLECTOR_IMAGE_DIGEST")
        else:
            errors.extend(validate(env_repository, env_digest, "environment overrides"))

    if errors:
        for error in errors:
            print(f"::error::{error}")
        return 1

    print("Collector image pinning parameters are valid.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
