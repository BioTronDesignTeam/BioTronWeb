# syntax=docker/dockerfile:1

FROM node:22-bookworm-slim AS build

WORKDIR /app

ARG VITE_API_URL
ARG VITE_IDP_URL
ENV VITE_API_URL=${VITE_API_URL}
ENV VITE_IDP_URL=${VITE_IDP_URL}

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
