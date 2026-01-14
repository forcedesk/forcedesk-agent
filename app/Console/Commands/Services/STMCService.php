<?php

namespace App\Console\Commands\Services;

use Illuminate\Console\Command;
use Laravel\Dusk\Browser;
use Facebook\WebDriver\Chrome\ChromeOptions;
use Facebook\WebDriver\Remote\RemoteWebDriver;
use Facebook\WebDriver\Remote\DesiredCapabilities;

class STMCService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:stmc-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Performs a synchronization of student data from the STMC (EduSTAR Management Console).';

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {
        if (empty(agent_config('emc.emc_username')) || empty(agent_config('emc.emc_password')) || empty(agent_config('emc.emc_school_code'))) {
            $this->error('Missing EMC configuration (username, password, or school code).');
            return false;
        }

        $username = agent_config('emc.emc_username');
        $password = agent_config('emc.emc_password');
        $schoolCode = agent_config('emc.emc_school_code');

        // URL-encode credentials for embedding in URL (handles special chars and backslash)
        $encodedUsername = rawurlencode($username);
        $encodedPassword = rawurlencode($password);
        $baseUrl = "https://{$encodedUsername}:{$encodedPassword}@stmc.education.vic.gov.au";

        $driver = null;

        try {
            // Set up Chrome options
            $options = new ChromeOptions();
            $options->addArguments([
                '--headless',
                '--disable-gpu',
                '--no-sandbox',
                '--disable-dev-shm-usage',
                '--ignore-certificate-errors',
                '--auth-server-whitelist=*stmc.education.vic.gov.au',
                '--auth-negotiate-delegate-whitelist=*stmc.education.vic.gov.au',
            ]);

            $capabilities = DesiredCapabilities::chrome();
            $capabilities->setCapability(ChromeOptions::CAPABILITY, $options);

            // Start ChromeDriver (assumes chromedriver is running on port 9515)
            $this->info('Starting browser...');
            $driver = RemoteWebDriver::create('http://localhost:9515', $capabilities);
            $browser = new Browser($driver);

            // Step 1: Initial login to STMC
            $this->info('Step 1: Logging in to STMC...');
            $browser->visit($baseUrl);
            $this->info('Login successful.');

            // Step 2: Select school from dropdown
            $this->info('Step 2: Selecting school with code: ' . $schoolCode);

            // Wait for page to load and find dropdown
            $browser->pause(2000);

            // Find and select school from dropdown using JavaScript
            $schoolSelected = $browser->script("
                var selects = document.querySelectorAll('select');
                var found = false;
                selects.forEach(function(select) {
                    var options = select.options;
                    for (var i = 0; i < options.length; i++) {
                        if (options[i].text.includes('{$schoolCode}')) {
                            select.selectedIndex = i;
                            select.dispatchEvent(new Event('change', { bubbles: true }));
                            found = true;
                            break;
                        }
                    }
                });
                return found;
            ");

            if (!empty($schoolSelected[0])) {
                $this->info('School selected successfully.');
            } else {
                $this->warn('Could not find school in dropdown, continuing anyway...');
            }

            // Submit the form if there's a submit button
            $browser->script("
                var form = document.querySelector('form');
                if (form) {
                    var submit = form.querySelector('button[type=\"submit\"], input[type=\"submit\"]');
                    if (submit) submit.click();
                    else form.submit();
                }
            ");

            $browser->pause(2000);

            // Step 3: Navigate to stud_pwd page
            $this->info('Step 3: Navigating to student password page...');
            $browser->visit($baseUrl . '/stud_pwd');
            $browser->pause(1000);
            $this->info('Student password page accessed.');

            // Step 4: Fetch student data from API
            $this->info('Step 4: Fetching student data...');
            $browser->visit($baseUrl . '/api/SchGetStuds?fullProps=true');
            $browser->pause(1000);

            // Get the page content (JSON response)
            $content = $browser->script("return document.body.innerText || document.body.textContent;");
            $jsonContent = $content[0] ?? '';

            // Output the JSON response to console
            $this->info('Student data retrieved successfully:');
            $decoded = json_decode($jsonContent, true);
            if ($decoded !== null) {
                $this->line(json_encode($decoded, JSON_PRETTY_PRINT));
            } else {
                $this->line($jsonContent);
            }

            return true;

        } catch (\Exception $e) {
            $this->error('Error: ' . $e->getMessage());
            return false;
        } finally {
            if ($driver) {
                $driver->quit();
            }
        }
    }
}
