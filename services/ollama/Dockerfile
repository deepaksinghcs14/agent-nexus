FROM ollama/ollama

RUN apt-get update && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/*

COPY start.sh /start.sh
RUN chmod +x /start.sh

ENTRYPOINT []
CMD ["/start.sh"]
