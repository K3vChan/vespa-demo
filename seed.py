#!/usr/bin/env python3
"""Seed documents into Vespa from feed.json."""

import argparse
import json
import sys
import urllib.request
import urllib.error

VESPA_BASE = "http://localhost:8080"


def parse_doc_id(put_id: str):
    """Parse 'id:films:film::1' into (namespace, doc_type, id)."""
    # format: id:<namespace>:<doc_type>::<user_specific>
    parts = put_id.split(":")
    return parts[1], parts[2], parts[4]


def delete_all(namespace: str, doc_type: str):
    """Delete all documents of a given type via the visit API."""
    url = f"{VESPA_BASE}/document/v1/{namespace}/{doc_type}/docid?wantedDocumentCount=100"
    deleted = 0

    while True:
        req = urllib.request.Request(url, method="GET")
        with urllib.request.urlopen(req) as resp:
            data = json.loads(resp.read())

        docs = data.get("documents", [])
        if not docs:
            break

        for doc in docs:
            doc_id = doc["id"].split("::")[-1]
            del_url = f"{VESPA_BASE}/document/v1/{namespace}/{doc_type}/docid/{doc_id}"
            del_req = urllib.request.Request(del_url, method="DELETE")
            urllib.request.urlopen(del_req)
            deleted += 1

        continuation = data.get("continuation")
        if not continuation:
            break
        url = f"{VESPA_BASE}/document/v1/{namespace}/{doc_type}/docid?wantedDocumentCount=100&continuation={continuation}"

    return deleted


def feed(feed_file: str, clean: bool):
    with open(feed_file) as f:
        documents = json.load(f)

    if not documents:
        print("No documents found in feed file.")
        return

    namespace, doc_type, _ = parse_doc_id(documents[0]["put"])

    if clean:
        print(f"Deleting existing {doc_type} documents...", end=" ", flush=True)
        n = delete_all(namespace, doc_type)
        print(f"{n} deleted.")

    total = len(documents)
    errors = 0

    for i, doc in enumerate(documents, 1):
        _, _, doc_id = parse_doc_id(doc["put"])
        url = f"{VESPA_BASE}/document/v1/{namespace}/{doc_type}/docid/{doc_id}"
        body = json.dumps({"fields": doc["fields"]}).encode()
        req = urllib.request.Request(url, data=body, method="POST",
                                    headers={"Content-Type": "application/json"})
        try:
            urllib.request.urlopen(req)
        except urllib.error.HTTPError as e:
            print(f"\n  ERROR feeding {doc_id}: {e.code} {e.read().decode()}")
            errors += 1
            continue

        print(f"\r  Fed {i}/{total}", end="", flush=True)

    print()
    if errors:
        print(f"Done with {errors} error(s).")
        sys.exit(1)
    else:
        print(f"Done. {total} documents fed.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Seed Vespa with film documents")
    parser.add_argument("file", nargs="?", default="feed.json", help="Feed JSON file (default: feed.json)")
    parser.add_argument("--clean", action="store_true", help="Delete all existing documents before feeding")
    args = parser.parse_args()
    feed(args.file, args.clean)
