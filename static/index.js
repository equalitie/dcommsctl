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
                r.json().then(tar => {
                    deploymentTarget.appendChild(renderTemplate("template-target", {
                        // FIXME: get ID of created target
                        ID: 0,
                        User: tar.User,
                        Host: tar.Host,
                        Port: tar.Port,
                        Domain: tar.Domain,
                        // FIXME: get this from backend!
                        ContainerBackend: "docker",
                    }));
                });

            }).finally(r => { 
                for (const i of is) {
                    i.disabled = false;
                }
            });
        });
    }
}());
