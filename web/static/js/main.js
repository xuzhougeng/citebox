document.addEventListener('DOMContentLoaded', () => {
    const path = window.location.pathname;

    if (path === '/' || path === '/index.html') {
        DashboardPage.init();
    }

    if (path === '/library' || path === '/library.html') {
        LibraryPage.init();
    }

    if (path === '/upload' || path === '/upload.html') {
        UploadPage.init();
    }

    if (path === '/manual' || path === '/manual.html') {
        ManualPage.init();
    }

    if (path === '/figures' || path === '/figures.html') {
        FiguresPage.init();
    }

    if (path === '/groups' || path === '/groups.html') {
        GroupsPage.init();
    }

    if (path === '/tags' || path === '/tags.html') {
        TagsPage.init();
    }

    if (path === '/notes' || path === '/notes.html') {
        NotesPage.init();
    }

    if (path === '/ai' || path === '/ai.html') {
        AIReaderPage.init();
    }

    if (path === '/settings' || path === '/settings.html') {
        SettingsPage.init();
    }
});
