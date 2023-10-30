<x-layouts.app>
    <div class="mb-1 font-bold">Showing {{ count($servers) }} servers</div>
    <table style="compact">
        @foreach($servers as $server)
            <tr>
                <td>
                    <span class="text-green-400">#{{ $server->id }}</span>
                </td>
                <td class="pl-1">
                    <div>{{ $server->name }} <span class="text-gray-500">({{ $server->host }})</span></div>
                </td>
                <td class="pl-1">
                    @if($server->connected)
                        <span class="text-green-500">● connected</span>
                    @else
                        <span class="text-gray-600">● connected</span>
                    @endif
                </td>
            </tr>
        @endforeach
    </table>
</x-layouts.app>
