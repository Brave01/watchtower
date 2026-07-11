// nav.js — 两个服务共用的顶部导航，通过 <script src="/common/nav.js"></script> 引入。
// 用法：在页面内放一个 <div id="scm-nav"></div>，脚本加载后会渲染导航条，
// 并通过 window.SCM_APP（值为 "logmonitor" 或 "dashboard"）判断当前应用高亮哪个入口。
(function () {
  var APPS = [
    { key: "logmonitor", label: "日志监控", href: "//" + location.hostname + ":8080/" },
    { key: "dashboard", label: "服务大盘", href: "//" + location.hostname + ":8081/" },
  ];

  function render() {
    var mount = document.getElementById("scm-nav");
    if (!mount) return;

    var current = window.SCM_APP || "";
    var linksHtml = APPS.map(function (app) {
      var activeClass = app.key === current ? " scm-nav__link--active" : "";
      return (
        '<a class="scm-nav__link' +
        activeClass +
        '" href="' +
        app.href +
        '">' +
        app.label +
        "</a>"
      );
    }).join("");

    mount.className = "scm-nav";
    mount.innerHTML =
      '<div class="scm-nav__brand">瞭望塔 · Watchtower</div>' +
      '<div class="scm-nav__links">' +
      linksHtml +
      "</div>" +
      '<div class="scm-nav__right">' +
      '<span class="scm-nav__user" id="scm-nav-user"></span>' +
      '<button class="scm-nav__logout" id="scm-nav-logout">退出登录</button>' +
      "</div>";

    fetch("/api/auth/me", { credentials: "same-origin" })
      .then(function (res) {
        return res.json();
      })
      .then(function (data) {
        var userEl = document.getElementById("scm-nav-user");
        if (userEl && data && data.authenticated) {
          userEl.textContent = data.username;
        }
      })
      .catch(function () {});

    var logoutBtn = document.getElementById("scm-nav-logout");
    if (logoutBtn) {
      logoutBtn.addEventListener("click", function () {
        fetch("/api/auth/logout", { method: "POST", credentials: "same-origin" }).then(
          function () {
            window.location.href = "/login";
          }
        );
      });
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", render);
  } else {
    render();
  }
})();
