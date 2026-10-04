# --- Stage 1: Build Frontend Assets ---
FROM docker.io/library/node:20-alpine AS builder
WORKDIR /app

# Cache dependencies using package-lock.json
COPY package.json package-lock.json ./
RUN npm ci --legacy-peer-deps

# Copy source and build
COPY . .
RUN npm run build

# --- Stage 2: Serve with Nginx ---
FROM docker.io/library/nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]