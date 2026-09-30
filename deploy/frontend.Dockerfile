# Production image of the frontend: the Vite build served by nginx (non-root).
# The nginx configuration is mounted by docker-compose.prod.yml, not baked in.
# Build context: ../frontend (see docker-compose.prod.yml).

FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Same-origin API: nginx proxies /api/* to the backend, so no CORS in production.
ARG VITE_API_URL=/api
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.30-alpine
COPY --from=build /app/dist /usr/share/nginx/html
