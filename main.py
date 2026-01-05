import logging
import os

from dotenv import load_dotenv
from flask import Flask, redirect, render_template, request, session, url_for

app = Flask(__name__)
load_dotenv()
app.secret_key = os.getenv("SECRET")


logging.basicConfig(
    format="%(asctime)s - %(levelname)s - %(message)s",
    level=logging.WARNING,
    datefmt="%Y-%m-%d %H:%M:%S",
    filename="debug.log",
)


if os.getenv("MAIL") is None or os.getenv("PASSWD") is None:
    logging.critical("Env variable not found!!!")
    exit()


# Bare Index
@app.get("/")
def home():
    return "hello i am a tiny pea!!!"


# Auths
@app.route("/login", methods=["GET", "POST"])
def login():
    if session.get("user-mail", None) == os.getenv("MAIL"):
        return redirect(url_for("dashboard"))

    if request.method == "POST":
        mail: str = request.form.get("usr-email")
        passwd: str = request.form.get("usr-passwd")

        if mail != os.getenv("MAIL"):
            return render_template(
                "login.html",
                context={
                    "showToast": True,
                    "type": "error",
                    "msg": "Invalid User Email",
                },
            )
        if passwd != os.getenv("PASSWD"):
            return render_template(
                "login.html",
                context={
                    "showToast": True,
                    "type": "error",
                    "msg": "Invalid User Password",
                },
            )
        session["user-mail"] = mail
        return redirect(url_for("dashboard"))

    return render_template(
        "login.html", context={"showToast": False, "type": "", "msg": ""}
    )


@app.get("/logout")
def logout():
    if session.get("user-mail", None) == None:
        return redirect(url_for("home"))
    session.pop("user-mail", None)
    return redirect(url_for("home"))


# Dashboard
@app.route("/dashboard", methods=["GET", "POST"])
def dashboard():
    if session.get("user-mail", None) != os.getenv("MAIL"):
        return redirect(url_for("login"))
    avtr: str = session.get("user-mail", "XD")[0:2].upper()
    return render_template("dashboard.html", context={"avtr": avtr})


app.run(host="0.0.0.0", port=8000, debug=True)
