-- Non-superuser role the platform uses to connect to the demo databases.
CREATE ROLE platform_app LOGIN PASSWORD 'platform_app' NOSUPERUSER NOCREATEDB NOCREATEROLE;

CREATE DATABASE toko OWNER platform_app;
CREATE DATABASE kampus OWNER platform_app;

\connect toko
SET ROLE platform_app;
CREATE TABLE customers (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name text NOT NULL,
  email text NOT NULL UNIQUE,
  credit_limit numeric(12,2) NOT NULL CHECK (credit_limit >= 0)
);
CREATE TABLE products (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name text NOT NULL,
  price numeric(12,2) NOT NULL CHECK (price >= 0),
  stock integer NOT NULL DEFAULT 0 CHECK (stock >= 0)
);
CREATE TABLE orders (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id bigint NOT NULL REFERENCES customers(id),
  ordered_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX orders_customer_id_idx ON orders(customer_id);
INSERT INTO customers (name, email, credit_limit) VALUES ('Ayu', 'ayu@example.test', 500000), ('Bima', 'bima@example.test', 300000);
INSERT INTO products (name, price, stock) VALUES ('Notebook', 25000, 40), ('Pen', 7000, 100);

\connect kampus
SET ROLE platform_app;
CREATE TABLE students (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nim text NOT NULL UNIQUE,
  name text NOT NULL,
  enrolled_at date NOT NULL DEFAULT current_date
);
CREATE TABLE courses (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code text NOT NULL UNIQUE,
  title text NOT NULL,
  credits smallint NOT NULL CHECK (credits BETWEEN 1 AND 6)
);
CREATE TABLE enrollments (
  student_id bigint NOT NULL REFERENCES students(id),
  course_id bigint NOT NULL REFERENCES courses(id),
  PRIMARY KEY (student_id, course_id)
);
CREATE INDEX enrollments_course_id_idx ON enrollments(course_id);
INSERT INTO students (nim, name) VALUES ('13522001', 'Citra'), ('13522002', 'Dimas');
INSERT INTO courses (code, title, credits) VALUES ('IF3110', 'Pengembangan Aplikasi Web', 4), ('IF2211', 'Strategi Algoritma', 3);
