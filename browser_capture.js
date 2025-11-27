// Browser Console Redirect Tracker
// Instructions:
// 1. Open browser Developer Tools (F12)
// 2. Go to Console tab
// 3. Paste this entire script and press Enter
// 4. Copy the output and save it as redirect_data.json
// 5. Use the Go program to convert it to markdown files

(function() {
    console.log('=== Redirect Tracker - Starting ===');
    console.log('Monitoring network requests...\n');

    // Store all requests
    window.redirectSteps = [];
    let stepNumber = 1;

    // Monitor fetch/XHR
    const originalFetch = window.fetch;
    const originalXHROpen = XMLHttpRequest.prototype.open;
    const originalXHRSend = XMLHttpRequest.prototype.send;

    // Override fetch
    window.fetch = async function(...args) {
        const [url, options = {}] = args;
        const method = options.method || 'GET';

        console.log(`[Step ${stepNumber}] Fetch Request: ${method} ${url}`);

        const step = {
            stepNumber: stepNumber++,
            url: url,
            method: method,
            requestHeaders: options.headers || {},
            timestamp: new Date().toISOString()
        };

        try {
            const response = await originalFetch.apply(this, args);

            step.statusCode = response.status;
            step.statusText = response.statusText;
            step.responseHeaders = {};

            // Capture response headers
            response.headers.forEach((value, key) => {
                step.responseHeaders[key] = value;
            });

            // Check for redirect
            if (response.redirected) {
                step.redirectLocation = response.url;
            }

            console.log(`  Status: ${response.status} ${response.statusText}`);
            if (step.redirectLocation) {
                console.log(`  Redirected to: ${step.redirectLocation}`);
            }

            window.redirectSteps.push(step);

            return response;
        } catch (error) {
            step.error = error.message;
            window.redirectSteps.push(step);
            console.error(`  Error: ${error.message}`);
            throw error;
        }
    };

    // Monitor Performance Navigation Timing
    function captureNavigationTiming() {
        const perfEntries = performance.getEntriesByType('navigation');
        if (perfEntries.length > 0) {
            const nav = perfEntries[0];
            console.log('\n=== Navigation Timing ===');
            console.log(`URL: ${nav.name}`);
            console.log(`Type: ${nav.type}`);
            console.log(`Redirect Count: ${nav.redirectCount}`);
            console.log(`Transfer Size: ${nav.transferSize} bytes`);
        }
    }

    // Monitor Resource Timing (for all requests)
    function captureResourceTiming() {
        const resources = performance.getEntriesByType('resource');
        console.log('\n=== Resource Requests ===');
        resources.forEach((resource, index) => {
            if (resource.name.includes('sso.') || resource.name.includes('tax.')) {
                console.log(`\n[Resource ${index + 1}]`);
                console.log(`  URL: ${resource.name}`);
                console.log(`  Type: ${resource.initiatorType}`);
                console.log(`  Duration: ${resource.duration.toFixed(2)}ms`);
                console.log(`  Transfer Size: ${resource.transferSize} bytes`);
            }
        });
    }

    console.log('\n=== Instructions ===');
    console.log('1. This script is now monitoring network requests');
    console.log('2. Navigate to the URL in the address bar or click a link');
    console.log('3. After the redirects complete, run: exportRedirectData()');
    console.log('4. Copy the JSON output and save it as redirect_data.json\n');

    // Export function
    window.exportRedirectData = function() {
        console.log('\n=== Captured Redirect Data ===\n');

        // Also capture from Performance API
        const perfNav = performance.getEntriesByType('navigation')[0];
        const perfResources = performance.getEntriesByType('resource')
            .filter(r => r.name.includes('sso.') || r.name.includes('tax.'));

        const data = {
            capturedAt: new Date().toISOString(),
            navigationInfo: perfNav ? {
                url: perfNav.name,
                type: perfNav.type,
                redirectCount: perfNav.redirectCount,
                duration: perfNav.duration,
                transferSize: perfNav.transferSize
            } : null,
            resources: perfResources.map(r => ({
                url: r.name,
                type: r.initiatorType,
                duration: r.duration,
                transferSize: r.transferSize
            })),
            steps: window.redirectSteps
        };

        const jsonOutput = JSON.stringify(data, null, 2);
        console.log(jsonOutput);
        console.log('\n=== End of Data ===\n');
        console.log('Copy the JSON above and save it as redirect_data.json');

        // Try to download automatically
        try {
            const blob = new Blob([jsonOutput], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'redirect_data.json';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            console.log('✓ File download triggered!');
        } catch (e) {
            console.log('⚠ Automatic download failed, please copy manually');
        }

        return data;
    };

    // Capture initial state
    setTimeout(() => {
        captureNavigationTiming();
        captureResourceTiming();
    }, 1000);

    console.log('✓ Redirect tracker initialized!');
})();
