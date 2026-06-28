FROM nginx:1.27-alpine
ARG VITE_API_BASE=/api
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY dist/ /usr/share/nginx/html/
RUN sed -i "s|__VITE_API_BASE__|${VITE_API_BASE}|g" /usr/share/nginx/html/index.html
