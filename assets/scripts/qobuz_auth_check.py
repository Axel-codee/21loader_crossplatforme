#!/usr/bin/env python3
import json
import sys

sys.dont_write_bytecode = True

from qobuz_common import load_client, run_with_qobuz_error_handling

JSON_MARKER = "__LOADER21_QOBUZ_JSON__"


def main():
    client = load_client()
    membership_label = str(getattr(client, "label", "") or "").strip()
    auth_mode = str(getattr(client, "auth_mode", "") or "").strip()
    user_auth_token = str(getattr(client, "refreshed_user_auth_token", "") or "").strip()
    print(
        JSON_MARKER
        + json.dumps(
            {
                "membership_label": membership_label,
                "auth_mode": auth_mode,
                "user_auth_token": user_auth_token,
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    run_with_qobuz_error_handling(main)
