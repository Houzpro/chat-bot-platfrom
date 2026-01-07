-- Users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for faster email lookups
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- Bots table
CREATE TABLE IF NOT EXISTS bots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    -- Bot configuration (JSON)
    config JSONB DEFAULT '{}',
    -- Generation parameters (can be overridden per request)
    temperature DECIMAL(3, 2) DEFAULT 0.75,
    top_p DECIMAL(3, 2) DEFAULT 0.92,
    top_k INTEGER DEFAULT 40,
    max_new_tokens INTEGER DEFAULT 512,
    do_sample BOOLEAN DEFAULT true,
    system_prompt TEXT,
    -- RAG settings (chunk configuration)
    chunk_size INTEGER DEFAULT 800,
    chunk_overlap INTEGER DEFAULT 200,
    -- Status
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for bots
CREATE INDEX IF NOT EXISTS idx_bots_owner_id ON bots(owner_id);
CREATE INDEX IF NOT EXISTS idx_bots_is_active ON bots(is_active);

-- Bot documents tracking (metadata only, actual vectors in Qdrant)
CREATE TABLE IF NOT EXISTS bot_documents (
    id SERIAL PRIMARY KEY,
    bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    file_type VARCHAR(50),
    file_size BIGINT,
    chunks_count INTEGER DEFAULT 0,
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_bot_documents_bot_id ON bot_documents(bot_id);

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_bots_updated_at BEFORE UPDATE ON bots
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
