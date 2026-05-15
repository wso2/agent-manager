"""Mocked API clients for the IT helpdesk agent.

Each module emulates the behavior of a real backend service while keeping all
state in-process. Tool functions in tools.py call into these clients exactly as
they would call real HTTP services, so swapping mocks for real backends is a
local change inside this package.
"""
