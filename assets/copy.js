(function () {
    "use strict";

    function fallbackCopy(text) {
        var input = document.createElement("textarea");
        input.value = text;
        input.setAttribute("readonly", "");
        input.style.position = "fixed";
        input.style.opacity = "0";
        document.body.appendChild(input);
        input.select();
        var copied = document.execCommand("copy");
        document.body.removeChild(input);
        if (!copied) {
            throw new Error("copy command failed");
        }
    }

    function copyPath(button) {
        var path = button.getAttribute("data-copy-path");
        var originalLabel = button.textContent;
        var copy = navigator.clipboard && window.isSecureContext
            ? navigator.clipboard.writeText(path)
            : Promise.resolve().then(function () {
                fallbackCopy(path);
            });

        copy.then(function () {
            button.textContent = "Copied";
            window.setTimeout(function () {
                button.textContent = originalLabel;
            }, 1500);
        }).catch(function () {
            button.textContent = "Copy failed";
            window.setTimeout(function () {
                button.textContent = originalLabel;
            }, 2000);
        });
    }

    document.querySelectorAll("[data-copy-path]").forEach(function (button) {
        button.addEventListener("click", function () {
            copyPath(button);
        });
    });
}());
