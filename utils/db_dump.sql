--
-- PostgreSQL database dump
--

-- Role: zobot
-- DROP ROLE IF EXISTS zobot;

CREATE ROLE zobot WITH
    LOGIN
    SUPERUSER
    INHERIT
    CREATEDB
    CREATEROLE
    NOREPLICATION
    ENCRYPTED PASSWORD 'SCRAM-SHA-256$4096:3mNYC6+Q7KfLe+f4qJQ+8wfsfsfsdffsdfsdfsvxcvxYY=';

-- Database: zobot

-- DROP DATABASE IF EXISTS zobot;

CREATE DATABASE zobot
    WITH
    OWNER = zobot
    ENCODING = 'UTF8'
    LC_COLLATE = 'en_US.utf8'
    LC_CTYPE = 'en_US.utf8'
    TABLESPACE = pg_default
    CONNECTION LIMIT = -1
    IS_TEMPLATE = False;

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: regexes; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.regexes (
    reg character varying NOT NULL
);


ALTER TABLE public.regexes OWNER TO postgres;

--
-- Name: subscribers; Type: TABLE; Schema: public; Owner: zobot
--

CREATE TABLE public.subscribers (
                                    chatid bigint NOT NULL,
                                    username character varying NOT NULL,
                                    isactive boolean NOT NULL
);


ALTER TABLE public.subscribers OWNER TO zobot;

--
-- Data for Name: regexes; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.regexes (reg) FROM stdin;
.*ЗАСМЕ.*
.*ОБОСРА.*
.*зАсМе.*
(?i).*засме.*
.*ЗАСМІЯВ.*
.*ЛЕГЕНДАРНЫЙ ТРЕД КОТОРЫЙ ВСЕХ БЕСИТ.*
\.


--
-- Name: subscribers subscribers_pkey; Type: CONSTRAINT; Schema: public; Owner: zobot
--

ALTER TABLE ONLY public.subscribers
    ADD CONSTRAINT subscribers_pkey PRIMARY KEY (chatid);


--
-- PostgreSQL database dump complete
--