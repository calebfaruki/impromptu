(function () {
    "use strict";
    var form = document.getElementById("search-form");
    var results = document.getElementById("results");
    if (!form || !results) return;

    form.addEventListener("submit", function (e) {
        e.preventDefault();
        var q = form.querySelector("input[name=q]").value;
        if (!q) return;

        fetch("/api/v1/search?q=" + encodeURIComponent(q))
            .then(function (r) { return r.json(); })
            .then(function (data) {
                if (!data.results || data.results.length === 0) {
                    results.innerHTML = "<p>No results found.</p>";
                    return;
                }
                var html = "<ul>";
                data.results.forEach(function (r) {
                    html += "<li><a href=\"/" + r.author + "/" + r.name + "\">"
                        + r.author + "/" + r.name + "</a>"
                        + "<br><span class=\"meta\">" + r.description + "</span></li>";
                });
                html += "</ul>";
                results.innerHTML = html;
            })
            .catch(function () {
                form.submit();
            });
    });
})();
