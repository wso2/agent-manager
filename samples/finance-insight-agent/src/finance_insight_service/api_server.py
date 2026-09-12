from __future__ import annotations

import argparse
import json
import os
import threading
import time
import uuid
from datetime import datetime, timezone
from enum import Enum
from typing import Any
from dotenv import load_dotenv
from flask import Flask, jsonify, request
from flask_cors import CORS
from openinference.instrumentation.crewai import CrewAIInstrumentor

# Disable CrewAI interactive tracing prompt that causes timeout in containerized environments
os.environ.setdefault("CREWAI_TRACING_ENABLED", "false")

from crewai.events import (
    CrewKickoffCompletedEvent,
    CrewKickoffFailedEvent,
    CrewKickoffStartedEvent,
    TaskCompletedEvent,
    TaskFailedEvent,
    TaskStartedEvent,
    crewai_event_bus,
)
from finance_insight_service.crew import FinanceInsightCrew


class JobStatus(str, Enum):
    """Enumerates async job lifecycle states."""
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


# In-memory job storage ; This sample assumes a single replica; multiple replicas will not share job state.
jobs = {}
jobs_lock = threading.Lock()
JOB_EXPIRATION_SECONDS = 600  # Jobs expire after 10 minutes (reduced to free memory faster)


def _utc_now() -> datetime:
    """Return the current UTC time."""
    return datetime.now(timezone.utc)


def _is_job_cancelled(job_id: str) -> bool:
    """Check whether the job has been cancelled."""
    with jobs_lock:
        job = jobs.get(job_id)
        return bool(job and job.get("status") == JobStatus.CANCELLED)


def _cleanup_expired_jobs():
    """Background thread to periodically clean up expired jobs."""
    while True:
        time.sleep(300)  # Check every 5 minutes
        try:
            now = _utc_now()
            expired_jobs = []
            
            with jobs_lock:
                for job_id, job in list(jobs.items()):
                    # Parse completion time
                    updated_str = job.get("updated_at", "")
                    if updated_str:
                        try:
                            updated_time = datetime.fromisoformat(updated_str.replace('Z', '+00:00'))
                        except (ValueError, TypeError):
                            continue
                        
                        age_seconds = (now - updated_time).total_seconds()
                        
                        # Remove completed/failed jobs older than expiration time
                        if job["status"] in [JobStatus.COMPLETED, JobStatus.FAILED, JobStatus.CANCELLED] and age_seconds > JOB_EXPIRATION_SECONDS:
                            expired_jobs.append(job_id)
                            del jobs[job_id]
                            print(f"[CLEANUP] Removed expired job {job_id} (age: {age_seconds:.0f}s)")
            
            if expired_jobs:
                import gc
                gc.collect()
                print(f"[CLEANUP] Cleaned up {len(expired_jobs)} expired jobs, freed memory")
        
        except Exception as e:
            print(f"[CLEANUP] Error in cleanup thread: {e}")


def _normalize_list(value: Any) -> list[str]:
    """Normalize list-like inputs into a list of non-empty strings."""
    if value is None:
        return []
    if isinstance(value, list):
        return [str(item).strip() for item in value if str(item).strip()]
    if isinstance(value, str):
        return [v.strip() for v in value.split(",") if v.strip()]
    return [str(value).strip()]


def _bounded_int(value: Any, default: int, min_v: int, max_v: int) -> int:
    """Parse an int and clamp to an allowed range, falling back to default."""
    try:
        v = int(value)
    except (TypeError, ValueError):
        return default
    return v if min_v <= v <= max_v else default


def _build_search_query(query: str, tickers: Any, sites: Any) -> str:
    """Build a SerpAPI search query with tickers and site filters."""
    parts = [query.strip()] if query.strip() else []
    tickers_list = _normalize_list(tickers)
    if tickers_list:
        parts.append("(" + " OR ".join(tickers_list) + ")")
    sites_list = _normalize_list(sites)
    if sites_list:
        parts.append("(" + " OR ".join(f"site:{s}" for s in sites_list) + ")")
    return " ".join(parts).strip()


def _format_task_label(task_name: str | None) -> str:
    """Format a task name into a user-facing label."""
    name = (task_name or "").lower()
    if "research" in name:
        return "Research"
    if "quant" in name:
        return "Quant"
    if "audit" in name:
        return "Audit"
    if "report" in name:
        return "Report"
    return (task_name or "Task").replace("_", " ").title()


def _extract_text(value: Any) -> str:
    """Extract text from common CrewAI output shapes."""
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        if "final_response" in value:
            return str(value["final_response"])
        if "report" in value:
            return str(value["report"])
    for attr in ("raw", "output", "json"):
        if hasattr(value, attr):
            try:
                extracted = getattr(value, attr)
            except Exception:
                continue
            if extracted:
                return str(extracted)
    return str(value)


def _extract_final_response(raw: Any) -> tuple[str, Any]:
    """Extract a final response string and parsed payload."""
    if raw is None:
        return "", raw
    text = _extract_text(raw)
    stripped = text.strip()
    if not stripped:
        return "", raw
    try:
        parsed = json.loads(stripped)
    except json.JSONDecodeError:
        return stripped, raw
    if isinstance(parsed, dict):
        if parsed.get("final_response"):
            return str(parsed["final_response"]), parsed
        if parsed.get("report"):
            return str(parsed["report"]), parsed
    return stripped, parsed


def _build_inputs(payload: dict[str, Any]) -> dict[str, Any]:
    """Build the input dictionary for the crew."""
    user_request = payload.get("message", "")
    query = payload.get("query") or user_request
    tickers = payload.get("tickers", "")
    sites = payload.get("sites", "")
    symbol = payload.get("symbol", "")
    interval = payload.get("interval", "1day")
    outputsize = _bounded_int(payload.get("outputsize"), 260, 10, 5000)
    horizon_days = _bounded_int(payload.get("horizon_days"), 30, 1, 365)
    provided_data = payload.get("provided_data", "")
    if isinstance(provided_data, (dict, list)):
        provided_data = json.dumps(provided_data)
    search_query = _build_search_query(query, tickers, sites)

    # Get current date/time to provide context
    current_date = datetime.now().strftime("%Y-%m-%d")
    current_year = datetime.now().year
    days = _bounded_int(payload.get("days"), 7, 1, 30)
    max_articles = _bounded_int(payload.get("max_articles"), 8, 1, 20)

    return {
        "user_request": user_request,
        "current_date": current_date,
        "current_year": current_year,
        "sources_requested": str(bool(payload.get("sources_requested"))),
        "query": query,
        "tickers": tickers,
        "sites": sites,
        "days": days,
        "max_articles": max_articles,
        "search_query": search_query,
        "symbol": symbol,
        "interval": interval,
        "outputsize": outputsize,
        "horizon_days": horizon_days,
        "request": user_request,
        "provided_data": provided_data,
    }


def create_app() -> Flask:
    """Create and configure the Flask app."""
    load_dotenv()
    CrewAIInstrumentor().instrument()

    app = Flask(__name__)
    allowed_origins_env = os.getenv("CORS_ALLOWED_ORIGINS", "").strip()
    if allowed_origins_env:
        allowed_origins = [
            origin.strip()
            for origin in allowed_origins_env.split(",")
            if origin.strip()
        ]
    else:
        allowed_origins = "*"
    CORS(app, resources={
        r"/*": {
            "origins": allowed_origins,
            "methods": ["GET", "POST", "OPTIONS"],
            "allow_headers": ["Content-Type", "Authorization", "X-API-Key"],
            "expose_headers": ["Content-Type"],
            "supports_credentials": False
        }
    })

    # Start cleanup thread (only once per app instance)
    if not hasattr(create_app, '_cleanup_started'):
        cleanup_thread = threading.Thread(target=_cleanup_expired_jobs, daemon=True)
        cleanup_thread.start()
        create_app._cleanup_started = True
        print("[INIT] Started job cleanup thread")

    api_key = os.getenv("API_KEY", "")

    def check_auth() -> bool:
        if not api_key:
            return True
        header = request.headers.get("Authorization", "")
        token = ""
        if header.lower().startswith("bearer "):
            token = header.split(" ", 1)[1].strip()
        token = token or request.headers.get("X-API-Key", "").strip()
        return token == api_key

    @app.before_request
    def _auth_guard():
        if request.path == "/health":
            return None
        if not check_auth():
            return jsonify({"error": "Unauthorized"}), 401
        return None

    @app.get("/health")
    def health():
        import sys
        job_count = 0
        pending = 0
        running = 0
        completed = 0
        failed = 0
        
        with jobs_lock:
            job_count = len(jobs)
            for job in jobs.values():
                status = job.get("status")
                if status == JobStatus.PENDING:
                    pending += 1
                elif status == JobStatus.RUNNING:
                    running += 1
                elif status == JobStatus.COMPLETED:
                    completed += 1
                elif status == JobStatus.FAILED:
                    failed += 1
        
        return jsonify(
            {
                "status": "ok",
                "jobs": {
                    "total": job_count,
                    "pending": pending,
                    "running": running,
                    "completed": completed,
                    "failed": failed,
                },
                "memory_mb": sys.getsizeof(jobs) / (1024 * 1024),
            }
        )

    @app.get("/config")
    def config():
        """Return which API services are configured"""
        has_serpapi = bool(os.getenv("SERPAPI_API_KEY"))
        has_news = has_serpapi
        return jsonify({
            "services": {
                "openai": bool(os.getenv("OPENAI_API_KEY")),
                "serpapi": has_serpapi,
                "twelveData": bool(os.getenv("TWELVE_DATA_API_KEY")),
                "alphaVantage": bool(os.getenv("ALPHAVANTAGE_API_KEY")),
            },
            "capabilities": {
                "news_search": has_news,
                "market_data": bool(os.getenv("TWELVE_DATA_API_KEY")),
                "fundamentals": bool(os.getenv("ALPHAVANTAGE_API_KEY")),
                "ai_agents": bool(os.getenv("OPENAI_API_KEY")),
            }
        })

    # ============= ASYNC JOB-BASED ENDPOINTS =============
    
    @app.post("/chat/async")
    def chat_async():
        """Start async report job and return job ID immediately."""
        payload = request.get_json(force=True) or {}
        message = str(payload.get("message", "")).strip()
        if not message:
            return jsonify({"error": "Empty message"}), 400

        # Create job
        job_id = str(uuid.uuid4())

        with jobs_lock:
            jobs[job_id] = {
                "id": job_id,
                "status": JobStatus.PENDING,
                "request": message,
                "traces": [],
                "result": None,
                "error": None,
                "created_at": _utc_now().isoformat(),
                "updated_at": _utc_now().isoformat(),
            }

        # Start background job
        def run_job():
            with jobs_lock:
                if jobs[job_id]["status"] == JobStatus.CANCELLED:
                    return
                jobs[job_id]["status"] = JobStatus.RUNNING
                jobs[job_id]["updated_at"] = _utc_now().isoformat()

            try:
                # Setup trace collection
                traces: list[dict[str, Any]] = []
                trace_lock = threading.Lock()
                seq = 0
                pending_crew_completed = False
                crew_completed_emitted = False
                seen_tasks: set[str] = set()

                def emit_trace(event_type: str, task: str | None = None, agent: str | None = None) -> None:
                    label = _format_task_label(task)
                    message_text = event_type.replace("_", " ").title()
                    if event_type == "crew_started":
                        message_text = "Workflow started"
                    elif event_type == "crew_completed":
                        message_text = "Workflow completed"
                    elif event_type == "crew_failed":
                        message_text = "Workflow failed"
                    elif event_type == "task_started":
                        message_text = f"{label} in progress"
                    elif event_type == "task_completed":
                        message_text = f"{label} completed"
                    elif event_type == "task_failed":
                        message_text = f"{label} failed"

                    with trace_lock:
                        nonlocal seq
                        seq += 1
                        entry = {
                            "seq": seq,
                            "type": event_type,
                            "message": message_text,
                            "agent": agent,
                            "task": task,
                            "timestamp": _utc_now().isoformat(),
                        }
                        traces.append(entry)
                        with jobs_lock:
                            jobs[job_id]["traces"] = traces[-10:]
                            jobs[job_id]["updated_at"] = _utc_now().isoformat()

                # Execute crew
                inputs = _build_inputs(payload)
                crew = FinanceInsightCrew(job_id=job_id).build_crew()

                try:
                    # Subscribe to events within a scoped handler context
                    def handler_wrapper(src, evt):
                        nonlocal pending_crew_completed
                        nonlocal crew_completed_emitted
                        evt_crew_name = getattr(evt, "crew_name", None)
                        if evt_crew_name and evt_crew_name != crew.name:
                            return
                        if isinstance(evt, CrewKickoffStartedEvent):
                            emit_trace("crew_started")
                        elif isinstance(evt, CrewKickoffCompletedEvent):
                            if crew_completed_emitted:
                                return
                            if "report_task" in seen_tasks:
                                emit_trace("crew_completed")
                                crew_completed_emitted = True
                            else:
                                pending_crew_completed = True
                        elif isinstance(evt, CrewKickoffFailedEvent):
                            emit_trace("crew_failed")
                        elif isinstance(evt, TaskStartedEvent):
                            task_name = getattr(evt.task, "name", None) or "task"
                            agent = getattr(getattr(evt.task, "agent", None), "role", None)
                            emit_trace("task_started", task=task_name, agent=agent)
                        elif isinstance(evt, TaskCompletedEvent):
                            task_name = getattr(evt.task, "name", None) or "task"
                            agent = getattr(getattr(evt.task, "agent", None), "role", None)
                            emit_trace("task_completed", task=task_name, agent=agent)
                            seen_tasks.add(task_name)
                            if task_name == "report_task" and not crew_completed_emitted:
                                emit_trace("crew_completed")
                                crew_completed_emitted = True
                                pending_crew_completed = False
                        elif isinstance(evt, TaskFailedEvent):
                            task_name = getattr(evt.task, "name", None) or "task"
                            agent = getattr(getattr(evt.task, "agent", None), "role", None)
                            emit_trace("task_failed", task=task_name, agent=agent)

                    with crewai_event_bus.scoped_handlers():
                        for event_cls in [
                            CrewKickoffStartedEvent,
                            CrewKickoffCompletedEvent,
                            CrewKickoffFailedEvent,
                            TaskStartedEvent,
                            TaskCompletedEvent,
                            TaskFailedEvent,
                        ]:
                            crewai_event_bus.on(event_cls)(handler_wrapper)

                        result = crew.kickoff(inputs=inputs)
                finally:
                    pass

                if _is_job_cancelled(job_id):
                    with jobs_lock:
                        jobs[job_id]["updated_at"] = _utc_now().isoformat()
                    return

                # Extract and save response
                final_response, raw_output = _extract_final_response(result)
                report_text = (final_response or "").strip()
                if not report_text:
                    raise ValueError("No report returned from crew.")

                # Update job with result
                with jobs_lock:
                    if jobs[job_id]["status"] == JobStatus.CANCELLED:
                        jobs[job_id]["updated_at"] = _utc_now().isoformat()
                        return
                    jobs[job_id]["status"] = JobStatus.COMPLETED
                    jobs[job_id]["result"] = {
                        "report": report_text,
                    }
                    jobs[job_id]["updated_at"] = _utc_now().isoformat()

            except Exception as e:
                import traceback
                traceback.print_exc()
                with jobs_lock:
                    if jobs[job_id]["status"] != JobStatus.CANCELLED:
                        jobs[job_id]["status"] = JobStatus.FAILED
                        jobs[job_id]["error"] = str(e)
                    jobs[job_id]["updated_at"] = _utc_now().isoformat()

        thread = threading.Thread(target=run_job, daemon=True)
        thread.start()

        return jsonify({"jobId": job_id, "status": JobStatus.PENDING})

    @app.get("/chat/async/<job_id>/status")
    def get_job_status(job_id: str):
        """Get job status and latest traces."""
        with jobs_lock:
            job = jobs.get(job_id)
            if not job:
                return jsonify({"error": "Job not found"}), 404
            
            return jsonify({
                "jobId": job["id"],
                "status": job["status"],
                "traces": job["traces"][-10:],  # Last 10 traces
                "traceCount": len(job["traces"]),
                "updatedAt": job["updated_at"],
            })

    @app.post("/chat/async/<job_id>/cancel")
    def cancel_job(job_id: str):
        """Cancel a running job (best effort)."""
        with jobs_lock:
            job = jobs.get(job_id)
            if not job:
                return jsonify({"error": "Job not found"}), 404

            if job["status"] in [JobStatus.COMPLETED, JobStatus.FAILED, JobStatus.CANCELLED]:
                return jsonify({"jobId": job_id, "status": job["status"]})

            job["status"] = JobStatus.CANCELLED
            job["error"] = "Cancelled by user"
            job["updated_at"] = _utc_now().isoformat()

        return jsonify({"jobId": job_id, "status": JobStatus.CANCELLED})

    @app.get("/chat/async/<job_id>/result")
    def get_job_result(job_id: str):
        """Get final job result."""
        with jobs_lock:
            job = jobs.get(job_id)
            if not job:
                return jsonify({"error": "Job not found"}), 404
            
            if job["status"] == JobStatus.PENDING or job["status"] == JobStatus.RUNNING:
                return jsonify({"error": "Job not yet completed", "status": job["status"]}), 425
            
            if job["status"] == JobStatus.FAILED:
                return jsonify({"error": job["error"], "status": job["status"]}), 500

            if job["status"] == JobStatus.CANCELLED:
                return jsonify({"error": job.get("error", "Job cancelled"), "status": job["status"]}), 409

            result = {
                "jobId": job["id"],
                "status": job["status"],
                "result": job["result"],
            }
            
            # Don't delete job immediately - let cleanup thread handle expiration
            # This allows retrying result fetch if needed and prevents memory pressure
            # Jobs will be auto-cleaned after JOB_EXPIRATION_SECONDS
            
            return jsonify(result)

    return app


def main() -> None:
    """CLI entry point for running the API server."""
    parser = argparse.ArgumentParser(description="Finance Insight API server.")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=5000)
    parser.add_argument("--debug", action="store_true")
    args = parser.parse_args()

    app = create_app()
    app.run(host=args.host, port=args.port, debug=args.debug)


if __name__ == "__main__":
    main()
