(async function () {
    // this gets <template> by id from DOM and replaces all occurencies of {{variableName}} with value from variables[variableName]
    function renderTemplate(templateId, variables) {
        const template = document.getElementById(templateId);
        if (!template) {
            console.error(`Template with ID "${templateId}" not found`);
            return;
        }

        const fragment = document.importNode(template.content, true);

        function processNode(node) {
            // On fragments just walk through all of those
            if (node.nodeType === Node.DOCUMENT_FRAGMENT_NODE) {
                for (let i = 0; i < node.childNodes.length; i++) {
                    processNode(node.childNodes[i]);
                }
                return;
            }
            if (node.nodeType === Node.TEXT_NODE) {
                node.textContent = node.textContent.replace(/\{\{(\w+)\}\}/g, (match, varName) => {
                    return variables.hasOwnProperty(varName) ? variables[varName] : match;
                });
            } else if (node.nodeType === Node.ELEMENT_NODE) {
                for (let i = 0; i < node.attributes.length; i++) {
                    const attr = node.attributes[i];
                    attr.value = attr.value.replace(/\{\{(\w+)\}\}/g, (match, varName) => {
                        return variables.hasOwnProperty(varName) ? variables[varName] : match;
                    });
                }
                for (let i = 0; i < node.childNodes.length; i++) {
                    processNode(node.childNodes[i]);
                }
            }
        }

        processNode(fragment);

        return fragment;
    }


    let deploymentTarget = document.querySelector(".deployment_targets_list");
    let targetsReq = await fetch(`/target/list`);

    let targetsRes = await targetsReq.json();

    for (const t in targetsRes) {
        if (targetsRes.hasOwnProperty(t)) {
            const tar = targetsRes[t];

            deploymentTarget.appendChild(renderTemplate("template-target", {
                ID: t,
                User: tar.User,
                Host: tar.Host,
                Port: tar.Port,
                Domain: tar.Domain,
                // FIXME: get this from backend!
                ContainerBackend: "docker",
            }));
        }
    }


    let checkBtns = document.querySelectorAll(".btn-target-check");
    for (const b of checkBtns) {
        b.addEventListener("click", async function(e) {
            e.preventDefault();

            let btn = e.target;
            btn.disabled = true;
            const targetRoot = btn.closest(".target-info");
            let targetID = targetRoot.dataset.id;
            let installProm = fetch(`/target/${targetID}/check`, { method: "POST" });
            await installProm.then(r => {}).finally(r => { 
                btn.disabled = false;
            });
        });
    }

    let startBtns = document.querySelectorAll(".btn-target-start");
    for (const b of startBtns) {
        b.addEventListener("click", async function(e) {
            e.preventDefault();

            let btn = e.target;
            btn.disabled = true;
            const targetRoot = btn.closest(".target-info");
            let targetID = targetRoot.dataset.id;
            let installProm = fetch(`/target/${targetID}/start`, { method: "POST" });
            await installProm.then(r => {}).finally(r => { 
                btn.disabled = false;
            });
        });
    }
    let stopBtns = document.querySelectorAll(".btn-target-stop");
    for (const b of stopBtns) {
        b.addEventListener("click", async function(e) {
            e.preventDefault();

            let btn = e.target;
            btn.disabled = true;
            const targetRoot = btn.closest(".target-info");
            let targetID = targetRoot.dataset.id;
            let installProm = fetch(`/target/${targetID}/stop`, { method: "POST" });
            await installProm.then(r => {}).finally(r => { 
                btn.disabled = false;
            });
        });
    }

    let createVPSForms = document.querySelectorAll(".form-create-vps");
    for (const f of createVPSForms) {
        f.addEventListener("submit", async function(e) {
            e.preventDefault();
            let form = new FormData(e.target);

            let is = e.target.querySelectorAll("input,button");

            for (const i of is) {
                i.disabled = true;
            }

            let createProm = fetch(`/target/create`, { method: "POST", body: form });

            await createProm.then(r => {
                return r.json().then(tar => {
                    deploymentTarget.appendChild(renderTemplate("template-target", {
                        // FIXME: get ID of created target
                        ID: tar.ID,
                        User: tar.User,
                        Host: tar.Host,
                        Port: tar.Port,
                        Domain: tar.Domain,
                        // FIXME: get this from backend!
                        ContainerBackend: "docker",
                    }));

                    return fetch(`/target/${tar.ID}/install`, { method: "POST" });
                });

            }).catch(err => {
                alert(err);
            }).finally(r => { 
                for (const i of is) {
                    i.disabled = false;
                }
            });
        });
    }
}());
