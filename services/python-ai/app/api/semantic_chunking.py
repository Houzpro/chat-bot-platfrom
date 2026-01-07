from fastapi import APIRouter, Request
from sentence_transformers import SentenceTransformer
from langchain_experimental.text_splitter import SemanticChunker
from langchain_community.embeddings import HuggingFaceEmbeddings
import nltk

router = APIRouter()
model = SentenceTransformer("intfloat/multilingual-e5-base")
embeddings = HuggingFaceEmbeddings(model_name="intfloat/multilingual-e5-base")

# Ensure punkt is downloaded for NLTK
nltk.download("punkt")

@router.post("/semantic-chunks")
async def semantic_chunks(request: Request):
    data = await request.json()
    text = data["text"]
    chunker = SemanticChunker(embeddings)
    chunks = chunker.split_text(text)
    return {"chunks": chunks}
