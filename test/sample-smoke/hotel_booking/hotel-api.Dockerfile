FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt && pip check
COPY . .
RUN mkdir -p /app/storage && chown 10001:10001 /app/storage
ENV PYTHONDONTWRITEBYTECODE=1
USER 10001
CMD ["uvicorn", "service:app", "--host", "0.0.0.0", "--port", "9091"]
