FROM python:3.11-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PORT=8001
WORKDIR /app/mcp-server
COPY mcp-server/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt
COPY agent/data.py agent/policy_reads.py /app/agent/
COPY mcp-server/server.py ./server.py
RUN useradd --uid 10001 --create-home agent
USER 10001
EXPOSE 8001
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8001/health', timeout=3)"
CMD ["python", "server.py"]
