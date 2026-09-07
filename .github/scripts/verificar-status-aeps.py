#!/usr/bin/env python3
"""Valida o inventário e os status canônicos dos AEPs principais."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
AEP_DIR = ROOT / "aep"
README = AEP_DIR / "README.md"
STATUSES = ("Draft", "Open", "In Progress", "Done", "Accepted", "Superseded", "Deprecated")
ALIASES = {
    "Draft": ("Draft", "Rascunho", "Proposto"),
    "Open": ("Open",),
    "In Progress": ("In Progress",),
    "Done": ("Done", "Concluído", "Implementado", "Implementada"),
    "Accepted": ("Accepted", "Aceito"),
    "Superseded": ("Superseded", "Obsoleto"),
    "Deprecated": ("Deprecated", "Cancelado", "Cancelada", "Desprezado", "Desprezada"),
}
ROW_RE = re.compile(
    r"^\| \[([^\]]+)\]\(([^)]+)\) \| .*? \| (.*?) \|$",
    re.MULTILINE,
)
STATUS_RE = re.compile(
    r"^\s*(?:>\s*)?(?:#{1,6}\s*)?(?:-\s*)?(?:\*\*)?(?:Status|Estado)(?:\*\*)?\s*:",
    re.IGNORECASE,
)


def canonical_status(text: str) -> str | None:
    for canonical in STATUSES:
        if any(re.search(rf"\b{re.escape(alias)}\b", text, re.IGNORECASE) for alias in ALIASES[canonical]):
            return canonical
    return None


def main() -> int:
    errors: list[str] = []
    rows = ROW_RE.findall(README.read_text(encoding="utf-8"))
    labels: list[str] = []

    for label, relative_path, index_cell in rows:
        labels.append(label)
        path = AEP_DIR / relative_path
        if not path.is_file():
            errors.append(f"{label}: arquivo ausente: {relative_path}")
            continue

        top_lines = path.read_text(encoding="utf-8").splitlines()[:10]
        status_line = next((line for line in top_lines if STATUS_RE.match(line)), None)
        document_status = canonical_status(status_line or "")
        index_status = canonical_status(index_cell)
        if document_status is None:
            errors.append(f"{label}: status canônico ausente nas 10 primeiras linhas")
        elif index_status != document_status:
            errors.append(
                f"{label}: índice={index_status!r}, documento={document_status!r}"
            )

    occupied_numbers = {label.split("-", 1)[0] for label in labels}
    inventory = re.search(
        r"contém \*\*(\d+) documentos principais\s*>\s*para (\d+) números ocupados",
        README.read_text(encoding="utf-8"),
    )
    if inventory is None:
        errors.append("README: declaração de inventário não encontrada")
    else:
        declared_documents, declared_numbers = map(int, inventory.groups())
        if declared_documents != len(rows):
            errors.append(
                f"README: declara {declared_documents} documentos, tabela tem {len(rows)}"
            )
        if declared_numbers != len(occupied_numbers):
            errors.append(
                f"README: declara {declared_numbers} números, tabela tem {len(occupied_numbers)}"
            )

    if errors:
        print("\n".join(f"ERRO: {error}" for error in errors), file=sys.stderr)
        return 1

    print(
        f"OK: {len(rows)} documentos principais, "
        f"{len(occupied_numbers)} números ocupados e status sincronizados."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
