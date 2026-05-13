######### OpenLLMetry Instrumentation Image #########

# Python runtime version — must match the agent's Python version (the installed
# packages, including compiled C extensions, are ABI-tied to it).
ARG PYTHON_VERSION=3.11
# traceloop-sdk (OpenLLMetry) version pinned into this image. One pinned version
# per AMP instrumentation image; bump it by building a new AMP instrumentation version.
ARG TRACELOOP_VERSION=0.60.0

############### STAGE 1 — BUILD INSTRUMENTATION PACKAGES ###############
FROM public.ecr.aws/docker/library/python:${PYTHON_VERSION}-slim AS otel-tracing
ARG TRACELOOP_VERSION

# Set the working directory where all instrumentation-related build files will be stored.
WORKDIR /instrumentation

# Copy requirements and install instrumentation-related packages
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt "traceloop-sdk==${TRACELOOP_VERSION}" --target packages

# #Copy the custom sitecustomize.py into the packages folder.
COPY sitecustomize.py packages/sitecustomize.py

############### STAGE 2 — FINAL RUNTIME IMAGE ###############
# Create final OpenLLMetry image
ARG PYTHON_VERSION=3.11
FROM public.ecr.aws/docker/library/python:${PYTHON_VERSION}-slim
WORKDIR /instrumentations

# Copy the built instrumentation packages from the builder stage.
COPY --from=otel-tracing /instrumentation/packages /instrumentations/otel-tracing

# Copy the setup script
COPY setup-instrumentation.py /usr/local/bin/setup-instrumentation.py
# Make the script executable so the container can run it as an entrypoint.
RUN chmod +x /usr/local/bin/setup-instrumentation.py

ENV INSTRUMENTATION_PROVIDER=otel-tracing
CMD ["python3", "/usr/local/bin/setup-instrumentation.py"]
