-- Migration: Remove rag_top_k column from bots table
-- Now using dynamic result count based on score_threshold only

BEGIN;

-- Remove rag_top_k column
ALTER TABLE bots DROP COLUMN IF EXISTS rag_top_k;

-- Update default chunk values for existing bots
UPDATE bots 
SET 
    chunk_size = 800,
    chunk_overlap = 200
WHERE chunk_size > 1000 OR chunk_size IS NULL;

COMMIT;
