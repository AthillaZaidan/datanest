-- Non-superuser role the platform uses to connect to the demo databases.
CREATE ROLE platform_app LOGIN PASSWORD 'platform_app' NOSUPERUSER NOCREATEDB NOCREATEROLE;

CREATE DATABASE toko OWNER platform_app;
CREATE DATABASE kampus OWNER platform_app;
