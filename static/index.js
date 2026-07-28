(async function () {

    let installVPSForms = document.querySelectorAll(".form-install-vps");
    for (const f of installVPSForms) {
        f.addEventListener("submit", async function(e) {
            e.preventDefault();
            let btn = e.target;

            btn.disabled = true;

            let targetID = form.dataset.id;

            let installProm = fetch(`/target/${targetID}/install`, { method: "POST", body: form });

            await installProm.then(r => {}).finally(r => { 
                btn.disabled = false;
            });
        });
    }

    let deploymentTarget = document.querySelector(".deployment_targets_list");

    let createVPSForms = document.querySelectorAll(".form-create-vps");
    for (const f of createVPSForms) {
        f.addEventListener("submit", async function(e) {
            e.preventDefault();
            let form = new FormData(e.target);

            let is = e.target.querySelectorAll("input,button");

            for (const i of is) {
                i.disabled = true;
            }

            // FIXME: implement deployment targets
            let targetID = 0;

            let installProm = fetch(`/target/create`, { method: "POST", body: form });

            await installProm.then(r => {
                r.json().then(r => {
                    deploymentTarget.innerHTML += `<div>${r.Host} ${r.Port}</div>`;
                });

            }).finally(r => { 
                for (const i of is) {
                    i.disabled = false;
                }
            });
        });
    }
}());
