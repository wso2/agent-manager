FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt && pip check
COPY . .
RUN mkdir -p /app/storage && chown 10001:10001 /app/storage
ENV PYTHONDONTWRITEBYTECODE=1
USER 10001
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["python", "-c", "from urllib.request import urlopen; urlopen('http://127.0.0.1:9091/health', timeout=4).close()"]

CMD ["uvicorn", "service:app", "--host", "0.0.0.0", "--port", "9091"]
