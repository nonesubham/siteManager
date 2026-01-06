from pathlib import Path


def initBKPDir(d_path: str) -> None:
    Path(d_path).mkdir(exist_ok=True)


def ListConf(d_path: str) -> list[str]:
    pth = Path(d_path)
    return [str(p) for p in pth.glob("*.conf")]


def extractServerName(conf_path: str) -> list[str]:
    return []

