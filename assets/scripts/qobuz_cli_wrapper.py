#!/usr/bin/env python3
import sys

sys.dont_write_bytecode = True

from qobuz_common import install_qobuz_dl_download_patch, install_qobuz_dl_token_patch, prepare_qobuz_dl_runtime_config


def main():
    install_qobuz_dl_token_patch()
    install_qobuz_dl_download_patch()
    prepare_qobuz_dl_runtime_config()

    import qobuz_dl

    return qobuz_dl.main()


if __name__ == "__main__":
    raise SystemExit(main())
