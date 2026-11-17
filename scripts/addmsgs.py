#!/usr/bin/env python3
"""Merge messages into the catalogs: addmsgs.py <json file of {key: [en, zh]}>."""
import json, sys, collections
pairs = json.load(open(sys.argv[1]), object_pairs_hook=collections.OrderedDict)
for idx, lang in enumerate(["en", "zh-Hans"]):
    path = f"internal/locales/{lang}.json"
    cat = json.load(open(path), object_pairs_hook=collections.OrderedDict)
    for k, v in pairs.items():
        cat[k] = v[idx]
    with open(path, "w") as f:
        json.dump(cat, f, ensure_ascii=False, indent=2)
        f.write("\n")
