(async function () {

    // GET INFO
    let infoResp = await fetch("/info")
    if (!infoResp.ok) {
        throw new Error(`Fetch problem ${infoResp.status}`);
    }
    document.getElementById("host-info").innerText = await infoResp.text();

    // REGISTER HANDLERS
    let envButton = document.querySelectorAll(".env-load-btn");
    for (const b of envButton) {
        b.addEventListener("click", async function(e) {
            let id = e.currentTarget.dataset["id"];
            let envResp = await fetch(`/env/${id}`);
            if (!envResp.ok) {
                throw new Error(`Fetch problem ${envResp.status}`);
            }
            document.getElementById("env-info").innerText = await envResp.text();
        });
    }

    let panicButton = document.querySelectorAll(".panic-btn");
    for (const b of panicButton) {
        b.addEventListener("click", async function(e) {
            let panicResp = await fetch(`/panic/`, { method: "POST" });
            if (!panicResp.ok) {
                throw new Error(`Panic problem ${panicResp.status}`);
            }
        });
    }

    let installVPSForms = document.querySelectorAll(".form-install-vps");
    for (const f of installVPSForms) {
        f.addEventListener("submit", async function(e) {
            e.preventDefault();
            let form = new FormData(e.target);

            let is = e.target.querySelectorAll("input,button");

            for (const i of is) {
                i.disabled = true;
            }

            let installProm = fetch(`/install`, { method: "POST", body: form });

            await installProm.then(r => {}).finally(r => { 
                for (const i of is) {
                    i.disabled = false;
                }
            });
        });
    }

    let configButton = document.querySelectorAll(".config-load-btn");
    for (const b of configButton) {
        b.addEventListener("click", async function(e) {
            let id = e.currentTarget.dataset["id"];
            let fileResp = await fetch(`/config/${id}`);

            if (!fileResp.ok) {
                throw new Error(`Fetch problem ${fileResp.status}`);
            }

            let result = await fileResp.text();
            let area = document.getElementById("editor-area");
            area.value = result;
            let configSaveButton = document.querySelectorAll(".config-save-btn");
            for (const b of configSaveButton) {
                b.dataset["id"] = id;
            }
        });
    }

    let configSaveButton = document.querySelectorAll(".config-save-btn");
    for (const b of configSaveButton) {
        b.addEventListener("click", async function(e) {
            let area = document.getElementById("editor-area");
            let data = new URLSearchParams();

            if (area.value != "") {
                data.append("file_contents", area.value);

                let id = e.currentTarget.dataset["id"];
                let saveResp = await fetch(`/config/${id}`, { method: "POST", body: data });
                if (!saveResp.ok) {
                    throw new Error(`Fetch problem ${saveResp.status}`);
                }

                area.value = "";
            }
        });
    }

    let startService = document.querySelectorAll(".service-start");
    for (const b of startService) {
        b.addEventListener("click", async function(e) {
            let id = e.currentTarget.dataset["id"];
            let resp = await fetch(`/service/${id}/start`, { method: "POST" });

            if (!resp.ok) {
                throw new Error(`Fetch problem ${resp.status}`);
            }
        });
    }

    let stopService = document.querySelectorAll(".service-stop");
    for (const b of stopService) {
        b.addEventListener("click", async function(e) {
            let id = e.currentTarget.dataset["id"];
            let resp = await fetch(`/service/${id}/stop`, { method: "POST" });

            if (!resp.ok) {
                throw new Error(`Fetch problem ${resp.status}`);
            }
        });
    }

    let wsSock = new WebSocket("ws://localhost:8080/ws");

    wsSock.onclose = function(e) {};

    wsSock.onmessage = function(e) {
        let res = e.data;
        let args = res.split(" ");

        if (args[0] === "service_up") {
            let serviceID = args[1];
            let el = document.querySelector(`.service[data-id=${serviceID}] .service-off`);

            if (el) {
                el.classList.remove("service-off");
                el.classList.add("service-on");
            }
        }

        if (args[0] === "service_down") {
            let serviceID = args[1];
            let el = document.querySelector(`.service[data-id=${serviceID}] .service-on`);
            if (el) {
                el.classList.remove("service-on");
                el.classList.add("service-off");
            }
        }
    };

}());


