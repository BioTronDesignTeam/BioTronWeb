# syntax=docker/dockerfile:1

FROM node:22-bookworm-slim

WORKDIR /app

COPY package*.json ./
RUN npm install

COPY . .

CMD ["npm", "run", "deploy"]
