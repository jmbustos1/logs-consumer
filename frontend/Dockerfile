# Dos etapas: build del sitio estático y servido con nginx. En AWS este contenedor no se usa
# —el `out/` va directo a S3 detrás de CloudFront—, pero en local sirve para levantar el panel
# igual que en producción, con una sola orden.
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY . .
# La URL de la API se hornea en el bundle: es una variable NEXT_PUBLIC_, o sea de build.
ARG NEXT_PUBLIC_API_URL=http://localhost:8787/api/v1
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /app/out /usr/share/nginx/html
EXPOSE 80
