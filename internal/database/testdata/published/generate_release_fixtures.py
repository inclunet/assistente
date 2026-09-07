"""Preenche e exporta schemas reconstruídos das tags publicadas.

Os arquivos de entrada são bancos vazios criados pelo AutoMigrate da própria
tag. Consulte README.md para o procedimento completo e verificável.
"""

from __future__ import annotations

import argparse
import sqlite3
from pathlib import Path

STAMP = "2026-01-02 03:04:05+00:00"
USER_A = "018f0000-0000-7000-8000-000000000001"
USER_B = "018f0000-0000-7000-8000-000000000002"


class Seeder:
    def __init__(self, connection: sqlite3.Connection) -> None:
        self.connection = connection
        self.columns: dict[str, set[str]] = {}

    def has_table(self, table: str) -> bool:
        row = self.connection.execute(
            "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?",
            (table,),
        ).fetchone()
        return row is not None

    def insert(self, table: str, **values: object) -> None:
        if not self.has_table(table):
            return
        columns = self.columns.setdefault(
            table,
            {row[1] for row in self.connection.execute(f"PRAGMA table_info(`{table}`)")},
        )
        filtered = {key: value for key, value in values.items() if key in columns}
        names = ", ".join(f"`{name}`" for name in filtered)
        placeholders = ", ".join("?" for _ in filtered)
        self.connection.execute(
            f"INSERT INTO `{table}` ({names}) VALUES ({placeholders})",
            tuple(filtered.values()),
        )


def uuid(number: int) -> str:
    return f"018f0000-0000-7000-8000-{number:012d}"


def common(values: dict[str, object], identifier: int) -> dict[str, object]:
    return {"id": uuid(identifier), "created_at": STAMP, "updated_at": STAMP, **values}


def canonicalize_create_table(statement: str) -> str:
    """Remove a ordem aleatória das constraints produzida pelo GORM."""
    if not statement.startswith("CREATE TABLE") or ",CONSTRAINT " not in statement:
        return statement
    if not statement.endswith(");"):
        raise ValueError(f"CREATE TABLE inesperado: {statement}")
    parts = statement[:-2].split(",CONSTRAINT ")
    columns = parts[0]
    constraints = sorted(f"CONSTRAINT {part}" for part in parts[1:])
    return f"{columns},{','.join(constraints)});"


def seed(path: Path, release: str, output: Path) -> None:
    connection = sqlite3.connect(path)
    connection.execute("PRAGMA foreign_keys = ON")
    seeder = Seeder(connection)

    seeder.insert(
        "users",
        **common(
            {
                "username": "fixture-a",
                "display_name": f"Pessoa A {release}",
                "password_hash": "fixture-sem-segredo",
                "role": "admin",
                "is_active": 1,
            },
            1,
        ),
    )
    seeder.insert(
        "users",
        **common(
            {
                "username": "fixture-b",
                "display_name": f"Pessoa B {release}",
                "password_hash": "fixture-sem-segredo",
                "role": "user",
                "is_active": 1,
            },
            2,
        ),
    )
    seeder.insert(
        "sessions",
        **common(
            {
                "user_id": USER_A,
                "refresh_token_hash": "hash-deterministico-a",
                "expires_at": "2030-01-02 03:04:05+00:00",
                "client_label": "fixture",
            },
            3,
        ),
    )

    for offset, user_id, suffix in ((0, USER_A, "a"), (100, USER_B, "b")):
        conversation = uuid(10 + offset)
        child_conversation = uuid(11 + offset)
        root_message = uuid(12 + offset)
        child_message = uuid(13 + offset)
        task_list = uuid(20 + offset)
        task = uuid(22 + offset)
        subtask = uuid(23 + offset)
        mcp_server = uuid(30 + offset)
        tool = uuid(32 + offset)
        pipeline = uuid(40 + offset)
        job = uuid(41 + offset)
        trigger = uuid(42 + offset)
        run = uuid(43 + offset)
        channel = uuid(50 + offset)
        external_id = f"contato-{suffix}"

        seeder.insert(
            "llm_providers",
            id=uuid(4 + offset),
            user_id=user_id,
            name=f"Provedor {suffix.upper()}",
            type="openai",
            api_format="openai",
            base_url=f"https://fixture.invalid/{suffix}",
            model="fixture-model",
            default_model="fixture-model",
            is_default=1,
            timeout=30,
            credential_pattern=f"fixture:{suffix}",
            auth_mode="none",
            reasoning_content_mode="disabled",
            created_at=STAMP,
            updated_at=STAMP,
        )
        seeder.insert(
            "conversations",
            **common(
                {
                    "user_id": user_id,
                    "title": f"Conversa {suffix.upper()} {release}",
                    "channel": "telegram",
                    "contact_id": external_id,
                    "summary": f"Resumo {suffix}",
                    "summarizing_in_progress": 0,
                },
                10 + offset,
            ),
        )
        seeder.insert(
            "conversations",
            **common(
                {
                    "user_id": user_id,
                    "title": f"Subconversa {suffix.upper()} {release}",
                    "kind": "subagent",
                    "parent_conversation_id": conversation,
                },
                11 + offset,
            ),
        )
        seeder.insert(
            "chat_messages",
            **common(
                {
                    "conversation_id": conversation,
                    "turn_id": root_message,
                    "role": "user",
                    "content": f"Mensagem raiz {suffix}",
                    "model": "fixture-model",
                    "source": "fixture",
                },
                12 + offset,
            ),
        )
        seeder.insert(
            "chat_messages",
            **common(
                {
                    "conversation_id": conversation,
                    "parent_id": root_message,
                    "turn_id": root_message,
                    "role": "assistant",
                    "content": f"Mensagem filha {suffix}",
                    "model": "fixture-model",
                    "source": "fixture",
                },
                13 + offset,
            ),
        )
        seeder.insert(
            "memory_records",
            **common(
                {
                    "user_id": user_id,
                    "content": f"Memória {suffix}",
                    "summary": f"Resumo memória {suffix}",
                    "load_policy": "retrievable",
                    "kind": "historical_note",
                    "scope": "user",
                    "tags": '["fixture"]',
                    "importance": 3,
                    "confidence": 80,
                },
                14 + offset,
            ),
        )
        seeder.insert(
            "credential_entries",
            **common(
                {
                    "user_id": user_id,
                    "pattern": f"fixture:{suffix}",
                    "auth_type": "none",
                    "token_enc": "",
                    "username": "",
                    "password_enc": "",
                    "headers_enc": "",
                    "expires_at": 0,
                    "refresh_token_enc": "",
                    "client_id_enc": "",
                    "client_secret_enc": "",
                },
                15 + offset,
            ),
        )
        seeder.insert(
            "task_lists",
            **common(
                {
                    "user_id": user_id,
                    "title": f"Lista {suffix.upper()}",
                    "slug": f"lista-{suffix}",
                    "description": f"Fixture {release}",
                    "preferred_view_mode": "list",
                    "validation_policy": "{}",
                    "custom_actions": "[]",
                    "conversation_id": conversation,
                },
                20 + offset,
            ),
        )
        seeder.insert(
            "task_list_workflows",
            **common(
                {
                    "task_list_id": task_list,
                    "statuses": '[{"id":1,"order":0,"label":"Aberta"}]',
                    "allowed_transitions": "{}",
                    "initial_status_id": 1,
                },
                21 + offset,
            ),
        )
        seeder.insert(
            "tasks",
            **common(
                {
                    "task_list_id": task_list,
                    "title": f"Tarefa {suffix}",
                    "description": "Tarefa raiz",
                    "code": f"FIX-{suffix.upper()}",
                    "status_id": 1,
                    "order": 1,
                    "conversation_id": conversation,
                },
                22 + offset,
            ),
        )
        seeder.insert(
            "tasks",
            **common(
                {
                    "task_list_id": task_list,
                    "title": f"Subtarefa {suffix}",
                    "description": "Tarefa filha",
                    "status_id": 1,
                    "parent_id": task,
                    "order": 2,
                },
                23 + offset,
            ),
        )
        seeder.insert(
            "task_notes",
            **common(
                {
                    "user_id": user_id,
                    "task_id": subtask,
                    "type": 1,
                    "content": f"Nota {suffix}",
                    "author_name": f"Pessoa {suffix.upper()}",
                    "author_id": user_id,
                    "external_source": "fixture",
                    "external_id": f"nota-{suffix}",
                },
                24 + offset,
            ),
        )
        seeder.insert(
            "mcp_servers",
            **common(
                {
                    "user_id": user_id,
                    "slug": f"mcp-{suffix}",
                    "name": f"MCP {suffix.upper()}",
                    "description": "Servidor sem rede",
                    "transport": "stdio",
                    "command": "fixture",
                    "args": "[]",
                    "env": "{}",
                    "auth_type": "none",
                    "enabled": 0,
                    "auto_connect": 0,
                },
                30 + offset,
            ),
        )
        seeder.insert(
            "mcp_server_logs",
            **common(
                {
                    "server_id": mcp_server,
                    "timestamp": STAMP,
                    "type": "fixture",
                    "message": f"Log {suffix}",
                    "data": "{}",
                },
                31 + offset,
            ),
        )
        seeder.insert(
            "tool_catalog",
            **common(
                {
                    "user_id": user_id,
                    "mcp_server_id": mcp_server,
                    "name": f"fixture_tool_{suffix}",
                    "display_name": f"Tool {suffix.upper()}",
                    "description": "Tool de fixture",
                    "origin": "mcp",
                    "category": "fixture",
                    "class": "read",
                    "risk": "low",
                    "schema": "{}",
                    "schema_hash": f"hash-{suffix}",
                    "schema_bytes": 2,
                    "tags": '["fixture"]',
                    "availability_status": "available",
                },
                32 + offset,
            ),
        )
        seeder.insert(
            "tool_invocations",
            **common(
                {
                    "user_id": user_id,
                    "tool_catalog_id": tool,
                    "origin_type": "job",
                    "origin_id": job,
                    "tool_call_id": f"call-{suffix}",
                    "status": "completed",
                    "dry_run": 1,
                    "input": "{}",
                    "output": "{}",
                    "metadata": "{}",
                    "retryable": 0,
                    "queued_at": STAMP,
                    "started_at": STAMP,
                    "completed_at": STAMP,
                    "duration_ms": 1,
                },
                35 + offset,
            ),
        )
        seeder.insert(
            "tags",
            **common(
                {
                    "user_id": user_id,
                    "slug": f"tag-{suffix}",
                    "name": f"Tag {suffix.upper()}",
                    "description": "Tag de fixture",
                    "color": "--accent",
                },
                33 + offset,
            ),
        )
        seeder.insert(
            "tag_assignments",
            **common(
                {
                    "user_id": user_id,
                    "tag_id": uuid(33 + offset),
                    "resource_type": "conversation",
                    "resource_id": conversation,
                },
                34 + offset,
            ),
        )
        seeder.insert(
            "job_pipelines",
            **common(
                {
                    "user_id": user_id,
                    "slug": f"pipeline-{suffix}",
                    "name": f"Pipeline {suffix.upper()}",
                    "description": "Pipeline de fixture",
                    "enabled": 0,
                    "metadata": "{}",
                },
                40 + offset,
            ),
        )
        seeder.insert(
            "jobs",
            **common(
                {
                    "user_id": user_id,
                    "pipeline_id": pipeline,
                    "slug": f"job-{suffix}",
                    "name": f"Job {suffix.upper()}",
                    "description": "Job desabilitado",
                    "enabled": 0,
                    "tool_catalog_id": tool,
                    "tool_name": f"fixture_tool_{suffix}",
                    "inputs": "{}",
                    "output_config": "{}",
                    "events_config": "{}",
                    "error_policy": "{}",
                    "max_runs_per_hour": 1,
                    "dry_run_config": "{}",
                    "created_by": user_id,
                },
                41 + offset,
            ),
        )
        seeder.insert(
            "job_triggers",
            **common(
                {
                    "user_id": user_id,
                    "job_id": job,
                    "type": "manual",
                    "enabled": 0,
                    "expression": "",
                    "config": "{}",
                },
                42 + offset,
            ),
        )
        seeder.insert(
            "job_runs",
            **common(
                {
                    "user_id": user_id,
                    "job_id": job,
                    "trigger_id": trigger,
                    "status": "completed",
                    "started_at": STAMP,
                    "completed_at": STAMP,
                    "duration_ms": 1,
                    "retry_count": 0,
                    "is_dry_run": 1,
                    "tool_name": f"fixture_tool_{suffix}",
                    "trigger_data": "{}",
                    "inputs": "{}",
                    "output": "{}",
                    "events_emitted": "[]",
                },
                43 + offset,
            ),
        )
        seeder.insert(
            "job_events",
            **common(
                {
                    "user_id": user_id,
                    "job_id": job,
                    "job_run_id": run,
                    "occurred_at": STAMP,
                    "type": "fixture",
                    "event": "completed",
                    "message": f"Evento {suffix}",
                    "data": "{}",
                },
                44 + offset,
            ),
        )
        seeder.insert(
            "job_run_events",
            **common(
                {
                    "user_id": user_id,
                    "job_run_id": run,
                    "sequence": 1,
                    "occurred_at": STAMP,
                    "type": "completed",
                    "message": f"Timeline {suffix}",
                    "data": "{}",
                },
                45 + offset,
            ),
        )
        seeder.insert(
            "channels",
            **common(
                {
                    "user_id": user_id,
                    "type": "telegram",
                    "slug": f"telegram-{suffix}",
                    "display_name": f"Telegram {suffix.upper()}",
                    "enabled": 0,
                    "profile": "default",
                    "max_history": 20,
                    "max_contacts": 10,
                    "settings": "{}",
                    "bot_token_ref": f"channel:telegram-{suffix}:bot_token",
                },
                50 + offset,
            ),
        )
        seeder.insert(
            "channel_contacts",
            **common(
                {
                    "user_id": user_id,
                    "channel_id": channel,
                    "external_id": external_id,
                    "display_name": f"Contato {suffix.upper()}",
                    "username": f"contato_{suffix}",
                    "authorized_at": STAMP,
                },
                51 + offset,
            ),
        )
        seeder.insert(
            "channel_contact_conversations",
            **common(
                {
                    "channel_id": channel,
                    "contact_external_id": external_id,
                    "conversation_id": conversation,
                },
                52 + offset,
            ),
        )
        seeder.insert(
            "channel_response_pending",
            conversation_id=conversation,
            channel="telegram",
            chat_id=external_id,
            audio_only=0,
            trace_id=f"trace-{suffix}",
            owner_user_id=user_id,
            reply_to_msg_id=f"reply-{suffix}",
            delivered_assistant_id=child_message,
            created_at=STAMP,
        )
        seeder.insert(
            "acp_sessions",
            **common(
                {
                    "user_id": user_id,
                    "conversation_id": conversation,
                    "provider_id": uuid(4 + offset),
                    "session_id": f"sessao-{suffix}",
                    "cwd": f"/fixture/{suffix}",
                },
                53 + offset,
            ),
        )
        seeder.insert(
            "sub_agent_runs",
            **common(
                {
                    "user_id": user_id,
                    "parent_conversation_id": conversation,
                    "parent_turn_id": root_message,
                    "child_conversation_id": child_conversation,
                    "turn_index": 1,
                    "status": "completed",
                    "background": 0,
                    "result_summary": f"Resultado {suffix}",
                    "assistant_message_id": child_message,
                    "chain_id": f"chain-{suffix}",
                    "chain_history": "[]",
                    "started_at": STAMP,
                    "completed_at": STAMP,
                    "delivered_at": STAMP,
                },
                54 + offset,
            ),
        )

    connection.execute("UPDATE schema_migrations SET applied_at = ?", (STAMP,))
    connection.execute("PRAGMA user_version = 9")
    connection.commit()
    violations = connection.execute("PRAGMA foreign_key_check").fetchall()
    if violations:
        raise RuntimeError(f"violações de chave estrangeira: {violations}")

    lines = [
        f"-- Reconstrução determinística do banco publicado {release}.",
        f"-- Gerada pelo AutoMigrate da tag {release}; dados sintéticos, sem PII.",
        "PRAGMA foreign_keys=OFF;",
        "BEGIN TRANSACTION;",
    ]
    dump = list(connection.iterdump())
    lines.extend(
        canonicalize_create_table(line)
        for line in dump
        if line not in {"BEGIN TRANSACTION;", "COMMIT;"}
        and not line.startswith("PRAGMA foreign_keys")
    )
    lines.extend(["COMMIT;", "PRAGMA user_version=9;", "PRAGMA foreign_keys=ON;", ""])
    output.write_text("\n".join(lines), encoding="utf-8", newline="\n")
    connection.close()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("release", choices=("0.2.0", "0.3.0", "0.4.0", "0.5.0"))
    parser.add_argument("database", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    seed(args.database, args.release, args.output)


if __name__ == "__main__":
    main()
