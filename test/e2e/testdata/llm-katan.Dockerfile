FROM python:3.12-slim
RUN useradd --create-home --shell /bin/bash appuser
RUN pip install --no-cache-dir llm-katan==0.7.3
USER appuser
ENTRYPOINT ["llm-katan"]
CMD ["--model", "test-model", "--backend", "echo", "--providers", "openai,azure_openai,anthropic", "--port", "8000"]
