<?php

namespace App\Console\Commands\Services;

use App\Models\Students;
use App\Models\User;
use GuzzleHttp\Client;
use Illuminate\Console\Command;
use function Laravel\Prompts\progress;

class PapercutService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:papercut-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Retrieves and sends Papercut data to tenant.';

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {

        $sdclient = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
        )]);

        $api_key = config('agentconfig.papercut.api_key');
        $api_url = config('agentconfig.papercut.api_url');

        $students = Students::whereNotNull('username')->orderby('username','asc')->get();
        $staff = User::whereNotNull('staff_code')->orderby('staff_code','asc')->get();

        if (!$students && !$staff) {
            $this->error('No staff or students found');
            return false;
        }

        $payload = [];

        $staffprogress = progress(label: 'Processing Staff Records', steps: count($staff));
        $staffprogress->start();

        foreach($staff as $object)
        {
            try {
                $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $object->staff_code . '</value>
                    </param>
                    <param>
                    <value>secondary-card-number</value>
                    </param>
                </params>
            </methodCall>
            ';

                $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                $client = new Client();

                $response = $client->request('POST', $api_url, $options);

                $xmlObject = simplexml_load_string($response->getbody());

                $jsonFormatData = json_encode($xmlObject);
                $result = json_decode($jsonFormatData, true);

                $data['username'] = $object->staff_code;
                $data['pin'] = $result['params']['param']['value'];

                /* Grab the Balance */
                $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $object->staff_code . '</value>
                    </param>
                    <param>
                    <value>balance</value>
                    </param>
                </params>
            </methodCall>
            ';

                $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                $client = new Client();

                $response = $client->request('POST', $api_url, $options);

                $xmlObject = simplexml_load_string($response->getbody());

                $jsonFormatData = json_encode($xmlObject);
                $result = json_decode($jsonFormatData, true);

                $data['balance'] = $result['params']['param']['value'];

                /* Push the Data to the array */
                if(is_numeric($data['balance']) && is_numeric($data['pin']))
                {
                    $payload['staff'][] = $data;
                }

                $staffprogress->advance();

            } catch (\Exception $e)
            {
                \Log::error('Could not get data for '.$object->staff_code);
            }

        }

        $staffprogress->finish();

        $studentprogress = progress(label: 'Processing Student Records', steps: count($students));
        $studentprogress->start();

        foreach($students as $student)
        {
            try {
                $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $student->username . '</value>
                    </param>
                    <param>
                    <value>secondary-card-number</value>
                    </param>
                </params>
            </methodCall>
            ';

                $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                $client = new Client();

                $response = $client->request('POST', $api_url, $options);

                $xmlObject = simplexml_load_string($response->getbody());

                $jsonFormatData = json_encode($xmlObject);
                $result = json_decode($jsonFormatData, true);

                $data['username'] = $student->username;
                $data['pin'] = $result['params']['param']['value'];

                /* Grab the Balance */
                $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $student->username . '</value>
                    </param>
                    <param>
                    <value>balance</value>
                    </param>
                </params>
            </methodCall>
            ';

                $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                $client = new Client();

                $response = $client->request('POST', $api_url, $options);

                $xmlObject = simplexml_load_string($response->getbody());

                $jsonFormatData = json_encode($xmlObject);
                $result = json_decode($jsonFormatData, true);

                $data['balance'] = $result['params']['param']['value'];

                /* Push the Data to the array */

                if(is_numeric($data['balance']) && is_numeric($data['pin']))
                {
                    $payload['students'][] = $data;
                }

                $studentprogress->advance();

            } catch (\Exception $e)
            {
                \Log::error('Could not get Papercut Data for '.$student->username);
            }

        }

        $studentprogress->finish();

        try {
            $this->info('Posting Data to '.config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/papercut-data');

            $response = $sdclient->post(config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/papercut-data', [
                'headers' => [],
                'body' => json_encode($payload),
            ]);

            $status = json_decode($response->getBody(), false);

            if($status->status != 'ok')
            {
                $this->error('There was an error sending the data to SchoolDesk!');
                $this->error($status->message);
            } else {
                $this->info('Posting the data succeeded!');
                $this->info($status->message);
            }

        } catch (\Exception $e)
        {
            \Log::error('Could not send data to SchoolDesk Tenant');
        }

    }
}
