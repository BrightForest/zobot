-- Remember chats that blocked the bot. Safe to run more than once.
ALTER TABLE public.subscribers
    ADD COLUMN IF NOT EXISTS blocked boolean NOT NULL DEFAULT false;
