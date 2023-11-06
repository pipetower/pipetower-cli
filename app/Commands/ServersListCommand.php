<?php

namespace App\Commands;

use LaravelZero\Framework\Commands\Command;
use Pipetower\PhpSdk\Pipetower;
use function Termwind\render;

class ServersListCommand extends Command
{
    /** @var string */
    protected $signature = 'servers:list';

    /** @var string */
    protected $description = 'List all servers';

    /**
     * Execute the console command.
     */
    public function handle(Pipetower $pt): void
    {
        $this->list($pt);
    }

    /**
     * Load the servers and display them
     */
    private function list(Pipetower $pt, int $page = 1): void
    {
        $response = $pt->servers->all(['page' => $page]);

        render(view('servers-list', [
            'servers' => $response['data'],
            'meta' => $response['meta'],
        ]));

        if ($response['meta']['has_more']) {
            $remaining = $response['meta']['total'] - $response['meta']['to'];
            if ($this->confirm("$remaining more server(s) available. Do you want to list the next ones?", true)) {
                $this->list($pt, $page + 1);
            }
        }
    }
}
