######### OpenLLMetry Instrumentation Image #########

############### STAGE 1 — BUILD INSTRUMENTATION PACKAGES ###############
FROM python:3.11-slim AS otel-tracing

# Set the working directory where all instrumentation-related build files will be stored.
WORKDIR /instrumentation

# Copy requirements and install instrumentation-related packages
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt --target packages

# #Copy the custom sitecustomize.py into the packages folder.
COPY sitecustomize.py packages/sitecustomize.py

############### STAGE 2 — FINAL RUNTIME IMAGE ###############
# Create final OpenLLMetry image
FROM python:3.11-slim
WORKDIR /instrumentations

# Copy the built instrumentation packages from the builder stage.
COPY --from=otel-tracing /instrumentation/packages /instrumentations/otel-tracing

# Copy the setup script
COPY setup-instrumentation.py /usr/local/bin/setup-instrumentation.py
# Make the script executable so the container can run it as an entrypoint.
RUN chmod +x /usr/local/bin/setup-instrumentation.py

ENV INSTRUMENTATION_PROVIDER=otel-tracing
CMD ["python3", "/usr/local/bin/setup-instrumentation.py"]
