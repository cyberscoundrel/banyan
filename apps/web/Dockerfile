# Dockerfile for building and serving the Banyan web app from local files
# Use this when you've already cloned the repository
#
# IMPORTANT: This Dockerfile MUST be built from the monorepo root, not from apps/web
# It requires access to monorepo-level files (tsconfig.base.json, nx.json, etc.)
#
# Build from monorepo root:
#   docker build -t banyan-web -f apps/web/Dockerfile .
#
# Run:
#   docker run -p 8080:80 banyan-web
#
# Or use docker-compose from apps/web directory:
#   cd apps/web && docker-compose up

# Stage 1: Build the application
FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files for dependency installation
COPY package.json package-lock.json ./
COPY apps/web/package.json ./apps/web/
COPY apps/web/package-lock.json* ./apps/web/

# Install root dependencies (for nx)
RUN npm ci --ignore-scripts

# Copy web app source files
COPY apps/web ./apps/web
COPY tsconfig.base.json ./
COPY nx.json ./

# Install web app dependencies and build
WORKDIR /app/apps/web
RUN npm ci
RUN npm run build

# Stage 2: Serve with nginx
FROM nginx:alpine AS production

# Copy custom nginx config for SPA routing
RUN rm /etc/nginx/conf.d/default.conf
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf

# Copy built assets
COPY --from=builder /app/apps/web/dist /usr/share/nginx/html

# Expose port 80
EXPOSE 80

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost/ || exit 1

CMD ["nginx", "-g", "daemon off;"]