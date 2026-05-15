"""Tool definitions exposed to the LangGraph IT helpdesk agent.

Each tool wraps a call into ``clients/`` and serializes results as plain strings
suitable for the LLM. Errors from the clients are returned as readable strings,
not raised, so the agent can decide what to do (typically: escalate).
"""

from __future__ import annotations

import json
from typing import Any

from langchain_core.tools import tool

from clients import employees as employees_client
from clients import escalations as escalations_client
from clients import passwords as passwords_client
from clients import policies as policies_client
from clients import software as software_client
from clients import system_status as status_client
from clients import tickets as tickets_client
from config import Config


def build_tools(cfg: Config) -> list[Any]:
    """Build the tool set bound to the given instance config."""

    @tool
    def lookup_employee(email: str) -> str:
        """Look up an employee by their email address.

        Returns the employee record (id, name, role, department, manager, admin status).
        Use this as the first step to identify who the requester is.
        """
        try:
            emp = employees_client.lookup_by_email(email)
            return json.dumps(emp)
        except employees_client.EmployeeNotFound as e:
            return f"ERROR: {e}"

    @tool
    def verify_identity(email: str, employee_id: str) -> str:
        """Verify an employee's identity by matching their email and employee ID.

        This MUST be called before any write action (password reset, access request,
        ticket creation). Returns the verified employee record.
        """
        try:
            emp = employees_client.verify(email, employee_id)
            return json.dumps(emp)
        except employees_client.EmployeeNotFound as e:
            return f"ERROR: {e}"

    @tool
    def get_open_tickets(employee_id: str) -> str:
        """List an employee's tickets (open, in-progress, and recent resolved).

        Use this to check for existing tickets before creating new ones
        (duplicate prevention policy).
        """
        rows = tickets_client.get_by_employee(employee_id)
        return json.dumps(rows[: cfg.max_tickets_per_query])

    @tool
    def create_ticket(
        employee_id: str,
        subject: str,
        category: str,
        priority: str,
        description: str,
    ) -> str:
        """Create a new IT support ticket.

        Categories: network, software, hardware, access, email, security, other.
        Priorities: P1 (critical), P2 (high), P3 (medium), P4 (low).

        Before creating, check for duplicate tickets and system status.
        Do not create a ticket if the issue matches a known outage.
        """
        dupes = tickets_client.find_duplicates(employee_id, category)
        if dupes:
            return json.dumps({
                "warning": "Potential duplicate ticket(s) found. Consider updating the existing ticket instead.",
                "existing_tickets": dupes,
            })
        ticket = tickets_client.create(
            employee_id=employee_id,
            subject=subject,
            category=category,
            priority=priority,
            description=description,
        )
        return json.dumps(ticket)

    @tool
    def reset_password(employee_id: str, is_admin: bool, verified: bool) -> str:
        """Reset an employee's password.

        Requires identity to be verified first (verified=true).
        Admin accounts CANNOT be reset — they must be escalated to L2 Security.
        On success, a temporary password is sent to the employee's registered email.
        """
        try:
            receipt = passwords_client.reset(
                employee_id=employee_id,
                is_admin=is_admin,
                verified=verified,
            )
            return json.dumps(receipt)
        except passwords_client.IdentityNotVerified as e:
            return f"ERROR: {e}"
        except passwords_client.AdminResetBlocked as e:
            return f"ERROR: {e} Use escalate_to_l2 to forward this request."

    @tool
    def request_software_access(
        employee_id: str,
        department: str,
        manager_id: str,
        software_name: str,
    ) -> str:
        """Request access to a software tool for an employee.

        Access is granted based on department eligibility. Some tools require
        manager approval. Requests outside the employee's department allowlist
        are denied.
        """
        try:
            result = software_client.request_access(
                employee_id=employee_id,
                department=department,
                manager_id=manager_id,
                software_name=software_name,
            )
            return json.dumps(result)
        except software_client.SoftwareNotFound as e:
            return f"ERROR: {e}"
        except software_client.DepartmentNotAllowed as e:
            return f"ERROR: {e}"
        except software_client.ApprovalRequired as e:
            return f"PENDING: {e}"

    @tool
    def check_system_status(service_query: str = "") -> str:
        """Check the current status of IT systems and services.

        Pass a service name to search (e.g. 'email', 'vpn') or leave empty
        to get all non-operational services. Always check this before creating
        tickets for connectivity or service issues.
        """
        if service_query.strip():
            results = status_client.check_service(service_query)
        else:
            results = status_client.get_degraded()
        if not results:
            return "All systems operational — no issues found matching your query."
        return json.dumps(results)

    @tool
    def search_it_policy(query: str) -> str:
        """Search the IT policy knowledge base for relevant policies.

        Use this when you need to cite a policy before taking an action
        or when answering policy-related questions.
        """
        docs = policies_client.search(query=query, limit=3)
        return json.dumps(docs)

    @tool
    def escalate_to_l2(employee_id: str, summary: str, priority: str, category: str) -> str:
        """Escalate a case to L2 support.

        Use this for: admin account issues, security incidents, infrastructure access,
        complex problems the L1 agent cannot resolve, or any situation requiring
        elevated privileges.

        Categories: security, infrastructure, admin_account, complex_issue.
        """
        ticket = escalations_client.create(
            employee_id=employee_id,
            summary=summary,
            priority=priority,
            category=category,
        )
        return json.dumps(ticket)

    return [
        lookup_employee,
        verify_identity,
        get_open_tickets,
        create_ticket,
        reset_password,
        request_software_access,
        check_system_status,
        search_it_policy,
        escalate_to_l2,
    ]
