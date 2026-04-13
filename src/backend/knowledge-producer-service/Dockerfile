FROM python:3.11-slim

WORKDIR /app

# Install dependencies
COPY backend/knowledge-producer-service/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY backend/knowledge-producer-service/src/ ./src/

# Copy questions and knowledge data files
COPY backend/common/questions/ /app/data/questions/
COPY backend/common/knowledge/ /app/data/knowledge/

# Set Python path
ENV PYTHONPATH=/app

# Expose port
EXPOSE 8092

# Run application
CMD ["python", "-m", "uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8092"]
